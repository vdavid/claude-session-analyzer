package session

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// Summary is one session as a listing sees it: what can be told from directory entries and the two ends of the lead
// transcript, without parsing a single body.
//
// That restraint is the point. The machine this was built against holds 4,438 transcripts and 3.8 GB of them, so a
// listing that decodes bodies is a listing nobody waits for.
type Summary struct {
	ID          string
	ProjectSlug string
	// ProjectPath is where the session started, read off the first record carrying a `cwd`. A session that entered a
	// worktree reports a different one further in, which is why the first wins.
	ProjectPath string
	// Title is what Claude Code shows: a title a person set beats a generated one, and the last of each kind wins.
	Title string
	// Start and End are the lead transcript's first and last timestamped records. A subagent could in principle outlive
	// the lead, and finding out would mean reading every lane, so End is the lead's.
	Start time.Time
	End   time.Time
	// Modified is the lead transcript's mtime, which is the honest answer for a session with no timestamped records.
	Modified time.Time
	// Lanes counts the subagent transcripts, the ones a workflow spawned included. The lead isn't one of them.
	Lanes int
	// Bytes is the lead transcript plus every lane's, which is what the session costs on disk.
	Bytes int64
}

// Duration is the session's wall clock, from its first record to its last.
func (s Summary) Duration() time.Duration { return s.End.Sub(s.Start) }

const (
	// headWindow and tailWindow are how much of a transcript's two ends a summary reads first. The head holds the first
	// record's `cwd` and timestamp; the tail holds the last timestamp and the titles, which get rewritten on most turns.
	// Both windows grow when what they're after isn't in them.
	headWindow = 16 << 10
	tailWindow = 64 << 10
	// titleWindow is as far as the tail grows looking for a title. A session rewrites its title on most turns, so a
	// quarter megabyte of tail holding none means the session never had one, not that the window is too small.
	titleWindow = 256 << 10
	// maxWindow stops a window growing forever on a transcript that carries one enormous line. The longest line in the
	// corpus is 3.42 MB, so a window past this one has already lost.
	maxWindow = 16 << 20
	// listValueBytes caps the payloads a listing decodes. It needs timestamps, a `cwd`, and titles, none of which are
	// capped, so anything larger is waste.
	listValueBytes = 64
)

// List summarizes every session under root, newest first.
//
// A project directory that can't be read is skipped rather than sinking the whole listing: transcripts are written
// while the tool runs, and one unreadable directory shouldn't cost a person the other 4,000 sessions.
func List(root string) ([]Summary, error) {
	slugs, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read the transcript root %s: %w", root, err)
	}

	type job struct {
		slug     string
		path     string
		summary  Summary
		leadSize int64
	}
	var jobs []job

	for _, slug := range slugs {
		if !slug.IsDir() {
			continue
		}
		dir := filepath.Join(root, slug.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || filepath.Ext(name) != ".jsonl" {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			id := strings.TrimSuffix(name, ".jsonl")
			jobs = append(jobs, job{
				slug:     slug.Name(),
				path:     filepath.Join(dir, name),
				leadSize: info.Size(),
				summary: Summary{
					ID:          id,
					ProjectSlug: slug.Name(),
					Modified:    info.ModTime(),
					Bytes:       info.Size(),
				},
			})
		}
	}

	// Every job is two reads and a couple of directory scans, so the wall clock is all latency. Spreading it over the
	// cores turns a listing of the whole corpus from seconds into a fraction of one.
	var wg sync.WaitGroup
	queue := make(chan int)
	workers := min(runtime.NumCPU(), len(jobs))
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range queue {
				j := &jobs[i]
				ends, err := summarizeFile(j.path, j.leadSize)
				if err == nil {
					j.summary.ProjectPath = ends.ProjectPath
					j.summary.Title = ends.Title
					j.summary.Start = ends.Start
					j.summary.End = ends.End
				}
				lanes, laneBytes := countLanes(strings.TrimSuffix(j.path, ".jsonl"))
				j.summary.Lanes = lanes
				j.summary.Bytes += laneBytes
			}
		}()
	}
	for i := range jobs {
		queue <- i
	}
	close(queue)
	wg.Wait()

	out := make([]Summary, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, j.summary)
	}
	sortNewestFirst(out)
	return out, nil
}

// Summarize reports one located session the same cheap way List does.
func Summarize(loc Location) (Summary, error) {
	info, err := os.Stat(loc.TranscriptPath)
	if err != nil {
		return Summary{}, fmt.Errorf("stat %s: %w", loc.TranscriptPath, err)
	}

	s, err := summarizeFile(loc.TranscriptPath, info.Size())
	if err != nil {
		return Summary{}, err
	}
	s.ID = loc.ID
	s.ProjectSlug = loc.ProjectSlug
	s.Modified = info.ModTime()
	s.Bytes = info.Size()

	lanes, laneBytes := countLanes(strings.TrimSuffix(loc.TranscriptPath, ".jsonl"))
	s.Lanes = lanes
	s.Bytes += laneBytes
	return s, nil
}

// sortNewestFirst puts the most recent session first, falling back to the file's mtime for a session whose records
// carry no timestamp at all.
func sortNewestFirst(sums []Summary) {
	when := func(s Summary) time.Time {
		if s.Start.IsZero() {
			return s.Modified
		}
		return s.Start
	}
	sort.SliceStable(sums, func(i, j int) bool {
		a, b := when(sums[i]), when(sums[j])
		if a.Equal(b) {
			return sums[i].ID < sums[j].ID
		}
		return a.After(b)
	})
}

func summarizeFile(path string, size int64) (Summary, error) {
	f, err := os.Open(path)
	if err != nil {
		return Summary{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only

	s, err := summarize(f, size)
	if err != nil {
		return Summary{}, fmt.Errorf("summarize %s: %w", path, err)
	}
	return s, nil
}

// summarize reads a transcript's two ends and reports what they say. It takes a ReaderAt rather than a path so a test
// can count the bytes it pulls, which is the property that matters more than any of the values.
func summarize(src io.ReaderAt, size int64) (Summary, error) {
	var s Summary
	if size <= 0 {
		return s, nil
	}

	// A transcript smaller than both windows is one read, not two overlapping ones.
	if size <= headWindow+tailWindow {
		recs, err := readRecords(src, 0, size, true, true)
		if err != nil {
			return s, err
		}
		s.readHead(recs)
		s.readTail(recs)
		return s, nil
	}

	head, err := headRecords(src, size)
	if err != nil {
		return s, err
	}
	s.readHead(head)

	tail, err := tailRecords(src, size)
	if err != nil {
		return s, err
	}
	s.readTail(tail)
	return s, nil
}

// readHead takes what only the start of a transcript can say.
func (s *Summary) readHead(recs []*transcript.Record) {
	for _, rec := range recs {
		if s.Start.IsZero() && !rec.Timestamp.IsZero() {
			s.Start = rec.Timestamp
		}
		if s.ProjectPath == "" && rec.CWD != "" {
			s.ProjectPath = rec.CWD
		}
		if !s.Start.IsZero() && s.ProjectPath != "" {
			return
		}
	}
}

// readTail takes what only the end of a transcript can say: the last timestamp, and the titles.
//
// End is the last timestamped record rather than the latest one, because compaction replays records under older stamps
// and a listing shouldn't be dragged around by whichever of them is furthest out.
func (s *Summary) readTail(recs []*transcript.Record) {
	var custom, ai, agent string
	for _, rec := range recs {
		if !rec.Timestamp.IsZero() {
			s.End = rec.Timestamp
		}
		switch rec.Type {
		case transcript.TypeCustomTitle:
			custom = rec.Title
		case transcript.TypeAITitle:
			ai = rec.Title
		case transcript.TypeAgentName:
			agent = rec.Title
		}
	}
	for _, title := range []string{custom, ai, agent} {
		if title != "" {
			s.Title = title
			return
		}
	}
}

// headRecords reads the start of a transcript, growing the window while it holds no complete line or nothing that
// carries a timestamp.
func headRecords(src io.ReaderAt, size int64) ([]*transcript.Record, error) {
	for window := int64(headWindow); ; window *= 4 {
		last := window >= size || window >= maxWindow
		if window > size {
			window = size
		}
		recs, err := readRecords(src, 0, window, true, window == size)
		if err != nil {
			return nil, err
		}
		if last || hasTimestamp(recs) {
			return recs, nil
		}
	}
}

// tailRecords reads the end of a transcript, growing the window for two reasons. A window holding no complete line has
// landed inside one line wider than itself, which happens: the widest line in the corpus is 3.42 MB. A window holding
// no title is probably too small, and that search stops at titleWindow.
func tailRecords(src io.ReaderAt, size int64) ([]*transcript.Record, error) {
	for window := int64(tailWindow); ; window *= 4 {
		last := window >= size || window >= maxWindow
		if window > size {
			window = size
		}
		recs, err := readRecords(src, size-window, window, window == size, true)
		switch {
		case err != nil:
			return nil, err
		case last, len(recs) > 0 && (hasTitle(recs) || window >= titleWindow):
			return recs, nil
		}
	}
}

// readRecords decodes the whole records inside one window of a transcript. A window that starts mid-file opens on a
// partial line and a window that ends mid-file closes on one; both are dropped, because half a line decodes to nothing
// useful and counts as malformed.
func readRecords(src io.ReaderAt, off, length int64, atStart, atEnd bool) ([]*transcript.Record, error) {
	buf := make([]byte, length)
	n, err := src.ReadAt(buf, off)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read %d bytes at %d: %w", length, off, err)
	}
	buf = buf[:n]

	if !atStart {
		i := bytes.IndexByte(buf, '\n')
		if i < 0 {
			return nil, nil
		}
		buf = buf[i+1:]
	}
	if !atEnd {
		i := bytes.LastIndexByte(buf, '\n')
		if i < 0 {
			return nil, nil
		}
		buf = buf[:i+1]
	}

	var recs []*transcript.Record
	r := transcript.NewReader(bytes.NewReader(buf), transcript.Options{MaxValueBytes: listValueBytes})
	for r.Next() {
		recs = append(recs, r.Record())
	}
	return recs, r.Err()
}

func hasTimestamp(recs []*transcript.Record) bool {
	for _, rec := range recs {
		if !rec.Timestamp.IsZero() {
			return true
		}
	}
	return false
}

func hasTitle(recs []*transcript.Record) bool {
	for _, rec := range recs {
		switch rec.Type {
		case transcript.TypeCustomTitle, transcript.TypeAITitle, transcript.TypeAgentName:
			if rec.Title != "" {
				return true
			}
		}
	}
	return false
}

// countLanes counts a session's subagent transcripts and adds up their size, reading directory entries only. sessionDir
// is the sibling directory named after the session; most sessions have none.
func countLanes(sessionDir string) (count int, bytes int64) {
	count, bytes = laneEntriesIn(filepath.Join(sessionDir, "subagents"))

	workflows := filepath.Join(sessionDir, "subagents", "workflows")
	entries, err := os.ReadDir(workflows)
	if err != nil {
		return count, bytes
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		n, b := laneEntriesIn(filepath.Join(workflows, entry.Name()))
		count += n
		bytes += b
	}
	return count, bytes
}

// laneEntriesIn counts the agent transcripts directly inside dir. Taking only `agent-*.jsonl` leaves out a workflow's
// `journal.jsonl`, which is the workflow's own log rather than a lane.
func laneEntriesIn(dir string) (count int, bytes int64) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "agent-") || filepath.Ext(name) != ".jsonl" {
			continue
		}
		count++
		if info, err := entry.Info(); err == nil {
			bytes += info.Size()
		}
	}
	return count, bytes
}
