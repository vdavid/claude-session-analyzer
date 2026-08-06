package session

import (
	"fmt"
	"os"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// Session is one parsed session: the lead lane plus one lane per subagent.
type Session struct {
	Location

	// Title is what Claude Code shows for the session. A title a person set wins over a generated one.
	Title string
	// ProjectPath is the directory the session started in, read off the first record that carries a `cwd`. The project
	// slug can't be inverted, and a session that entered a worktree reports a different `cwd` further in.
	ProjectPath string
	// Lanes hold the lead first, then the subagents in file-name order.
	Lanes []*Lane
}

// Lead returns the lead lane.
func (s *Session) Lead() *Lane {
	if len(s.Lanes) == 0 {
		return nil
	}
	return s.Lanes[0]
}

// Load reads every lane of a located session into memory. Parsing is cheap enough that nothing is cached; opts caps
// how much of each payload is kept.
func Load(loc Location, opts transcript.Options) (*Session, error) {
	s := &Session{Location: loc}

	lead, err := loadLane([]string{loc.TranscriptPath}, opts)
	if err != nil {
		return nil, err
	}
	lead.ID = loc.ID
	lead.Name = "lead"
	lead.IsLead = true
	s.Lanes = append(s.Lanes, lead)

	for _, files := range loc.SubagentLanes {
		lane, err := loadLane(files.Paths, opts)
		if err != nil {
			return nil, err
		}
		lane.ID = agentIDFromPath(files.Rel)
		lane.WorkflowID = workflowIDFromPath(files.Rel)
		lane.Meta = readAgentMeta(files.Paths)
		lane.Name = laneName(lane.Meta, lane.ID)
		s.Lanes = append(s.Lanes, lane)
	}

	s.Title = titleOf(lead)
	s.ProjectPath = projectPathOf(lead)
	return s, nil
}

// loadLane reads a lane. It's one file almost always, and several when the session moved between project directories
// while the lane was running: the harness carries on writing under the new slug, so the lane arrives in fragments.
func loadLane(paths []string, opts transcript.Options) (*Lane, error) {
	lane := &Lane{Paths: paths}
	frags := make([][]*transcript.Record, 0, len(paths))
	for _, path := range paths {
		recs, stats, err := readLaneFile(path, opts)
		if err != nil {
			return nil, err
		}
		frags = append(frags, recs)
		addStats(&lane.Stats, stats)
	}
	lane.Records = mergeFragments(frags)
	return lane, nil
}

func readLaneFile(path string, opts transcript.Options) ([]*transcript.Record, transcript.Stats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, transcript.Stats{}, fmt.Errorf("open transcript: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only

	var recs []*transcript.Record
	r := transcript.NewReader(f, opts)
	for r.Next() {
		recs = append(recs, r.Record())
	}
	if err := r.Err(); err != nil {
		return nil, transcript.Stats{}, fmt.Errorf("read %s: %w", path, err)
	}
	return recs, r.Stats(), nil
}

// mergeFragments puts a lane's fragments back into the order its records happened in, leaving each fragment's own order
// alone. The harness appends to whichever directory the session is in at the time and switches back and forth, so the
// fragments interleave: concatenating them puts records before the records they answer. Sorting the lot instead would
// reorder the backwards timestamps inside a fragment, which compaction produces on purpose, so the merge only ever
// chooses between fragments. A record carrying no timestamp travels with the one before it in its fragment.
func mergeFragments(frags [][]*transcript.Record) []*transcript.Record {
	if len(frags) == 1 {
		return frags[0]
	}

	total := 0
	keys := make([][]time.Time, len(frags))
	for i, frag := range frags {
		total += len(frag)
		keys[i] = make([]time.Time, len(frag))
		var last time.Time
		for j, rec := range frag {
			if !rec.Timestamp.IsZero() {
				last = rec.Timestamp
			}
			keys[i][j] = last
		}
	}

	at := make([]int, len(frags))
	out := make([]*transcript.Record, 0, total)
	for range total {
		next := -1
		for i := range frags {
			if at[i] == len(frags[i]) {
				continue
			}
			if next < 0 || keys[i][at[i]].Before(keys[next][at[next]]) {
				next = i
			}
		}
		out = append(out, frags[next][at[next]])
		at[next]++
	}
	return out
}

// addStats accumulates one fragment's reader stats into the lane's. The first malformed line is the first one found,
// which is the fragment order the files were read in.
func addStats(total *transcript.Stats, one transcript.Stats) {
	total.Lines += one.Lines
	total.Decoded += one.Decoded
	total.Skipped += one.Skipped
	total.Blank += one.Blank
	total.Malformed += one.Malformed
	if one.LongestLine > total.LongestLine {
		total.LongestLine = one.LongestLine
	}
	if total.FirstMalformedErr == nil && one.FirstMalformedErr != nil {
		total.FirstMalformedLine, total.FirstMalformedErr = one.FirstMalformedLine, one.FirstMalformedErr
	}
	if len(one.SkippedTypes) == 0 {
		return
	}
	if total.SkippedTypes == nil {
		total.SkippedTypes = map[string]int{}
	}
	for kind, n := range one.SkippedTypes {
		total.SkippedTypes[kind] += n
	}
}

// titleOf picks the session's title. All three kinds get rewritten as the session goes, so the last of each wins, and
// a title a person set beats a generated one.
func titleOf(lead *Lane) string {
	var custom, ai, agent string
	for _, rec := range lead.Records {
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
			return title
		}
	}
	return ""
}

func projectPathOf(lead *Lane) string {
	for _, rec := range lead.Records {
		if rec.CWD != "" {
			return rec.CWD
		}
	}
	return ""
}
