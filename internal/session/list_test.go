package session

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const betaTwoID = "33333333-bbbb-4bbb-8bbb-bbbbbbbbbbbb"

func at(t *testing.T, layout string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, layout)
	if err != nil {
		t.Fatalf("parse %q: %v", layout, err)
	}
	return ts.UTC()
}

func TestListReportsEverySessionUnderTheRoot(t *testing.T) {
	got, err := List(testRoot())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	byID := map[string]Summary{}
	for _, s := range got {
		byID[s.ID] = s
	}
	if len(byID) != 4 {
		t.Fatalf("listed %d sessions, want the 4 in testdata: %v", len(byID), byID)
	}

	alpha := byID[alphaID]
	if alpha.ProjectSlug != "-tmp-alpha" {
		t.Errorf("slug = %q", alpha.ProjectSlug)
	}
	// The session moved into a worktree partway through, so the first record's cwd is the one to report.
	if alpha.ProjectPath != "/tmp/alpha" {
		t.Errorf("project path = %q, want the first record's cwd", alpha.ProjectPath)
	}
	// A title a person set beats a generated one, whichever came last in the file.
	if alpha.Title != "Widgets, counted" {
		t.Errorf("title = %q, want the custom title", alpha.Title)
	}
	if want := at(t, "2026-08-03T08:44:00Z"); !alpha.Start.Equal(want) {
		t.Errorf("start = %s, want %s", alpha.Start, want)
	}
	if want := at(t, "2026-08-03T08:44:05Z"); !alpha.End.Equal(want) {
		t.Errorf("end = %s, want %s", alpha.End, want)
	}
	if alpha.Duration() != 5*time.Second {
		t.Errorf("duration = %s, want 5s", alpha.Duration())
	}
	// Three direct subagents plus the one a workflow spawned. The workflow's own journal isn't a lane.
	if alpha.Lanes != 4 {
		t.Errorf("lanes = %d, want 4", alpha.Lanes)
	}
	if alpha.Bytes <= 0 {
		t.Errorf("bytes = %d, want the lead transcript plus its lanes", alpha.Bytes)
	}
	if alpha.Modified.IsZero() {
		t.Error("modified should carry the lead transcript's mtime")
	}
}

func TestListSortsNewestFirst(t *testing.T) {
	got, err := List(testRoot())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	want := []string{betaTwoID, betaOneD, soloID, alphaID}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("order = %s, want %s (newest start first)", ids(got), strings.Join(want, ", "))
		}
	}
}

func TestListLeavesOutSessionsWithNoSubagents(t *testing.T) {
	got, err := List(testRoot())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, s := range got {
		if s.ID == soloID && s.Lanes != 0 {
			t.Errorf("lanes = %d for a session that spawned none", s.Lanes)
		}
	}
}

func TestListSkipsWakatimeSiblings(t *testing.T) {
	got, err := List(testRoot())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, s := range got {
		if strings.HasSuffix(s.ID, ".jsonl") {
			t.Errorf("%q came from a `.jsonl.wakatime` sibling, which belongs to another tool", s.ID)
		}
	}
}

func TestListReportsAMissingRoot(t *testing.T) {
	if _, err := List(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("want an error for a root that isn't there")
	}
}

// countingReaderAt counts what a summary actually pulls off disk.
type countingReaderAt struct {
	f    *os.File
	read atomic.Int64
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	n, err := c.f.ReadAt(p, off)
	c.read.Add(int64(n))
	return n, err
}

// TestSummarizeReadsOnlyTheTwoEndsOfATranscript is the listing's whole reason for existing: 4,438 transcripts and
// 3.8 GB of them means a listing that reads bodies is a listing nobody runs.
func TestSummarizeReadsOnlyTheTwoEndsOfATranscript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.jsonl")
	writeBigTranscript(t, path, 8<<20)

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close() //nolint:errcheck // read-only

	info, err := f.Stat()
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	counter := &countingReaderAt{f: f}

	got, err := summarize(counter, info.Size())
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}

	if got.ProjectPath != "/tmp/big" {
		t.Errorf("project path = %q", got.ProjectPath)
	}
	if got.Title != "The last title wins" {
		t.Errorf("title = %q, want the last one in the file", got.Title)
	}
	if want := at(t, "2026-08-01T00:00:00Z"); !got.Start.Equal(want) {
		t.Errorf("start = %s, want %s", got.Start, want)
	}
	if want := at(t, "2026-08-02T00:00:00Z"); !got.End.Equal(want) {
		t.Errorf("end = %s, want %s", got.End, want)
	}

	read := counter.read.Load()
	if ceiling := int64(headWindow + tailWindow); read > ceiling {
		t.Errorf("read %d bytes of an %d-byte transcript, want no more than %d", read, info.Size(), ceiling)
	}
}

// TestSummarizeReadsASmallTranscriptOnce keeps the windows from double-reading a file smaller than both of them.
func TestSummarizeReadsASmallTranscriptOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "small.jsonl")
	writeBigTranscript(t, path, 0)

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close() //nolint:errcheck // read-only

	info, err := f.Stat()
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	counter := &countingReaderAt{f: f}

	got, err := summarize(counter, info.Size())
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if got.Title != "The last title wins" {
		t.Errorf("title = %q", got.Title)
	}
	if read := counter.read.Load(); read > info.Size() {
		t.Errorf("read %d bytes of a %d-byte transcript, want it read once", read, info.Size())
	}
}

// TestSummarizeGrowsPastAMegabyteLine covers the shape that breaks a fixed window: a tail whose last line is longer
// than the window, so the first read comes back with no complete line in it.
func TestSummarizeGrowsPastAMegabyteLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wide.jsonl")
	var b strings.Builder
	b.WriteString(`{"type":"user","timestamp":"2026-08-01T00:00:00Z","cwd":"/tmp/wide","message":{"role":"user","content":"go"}}` + "\n")
	wider := ((headWindow + tailWindow) * 3) / 2
	b.WriteString(fmt.Sprintf(`{"type":"assistant","timestamp":"2026-08-02T00:00:00Z","message":{"role":"assistant","content":[{"type":"text","text":%q}]}}`, strings.Repeat("x", wider)) + "\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close() //nolint:errcheck // read-only
	info, err := f.Stat()
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	got, err := summarize(&countingReaderAt{f: f}, info.Size())
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if want := at(t, "2026-08-02T00:00:00Z"); !got.End.Equal(want) {
		t.Errorf("end = %s, want %s: the window has to grow past a line wider than itself", got.End, want)
	}
}

func TestSummarizeHandlesAnEmptyTranscript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close() //nolint:errcheck // read-only

	got, err := summarize(&countingReaderAt{f: f}, 0)
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if !got.Start.IsZero() || !got.End.IsZero() || got.Title != "" {
		t.Errorf("summary = %+v, want an empty one", got)
	}
}

// writeBigTranscript writes a transcript whose interesting records sit at the two ends, with filler between them, so a
// reader that skips the middle still gets everything and a reader that doesn't gets caught.
func writeBigTranscript(t *testing.T, path string, filler int) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close() //nolint:errcheck // closed again below

	write := func(line string) {
		if _, err := io.WriteString(f, line+"\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	write(`{"type":"user","uuid":"h1","timestamp":"2026-08-01T00:00:00.000Z","cwd":"/tmp/big","message":{"role":"user","content":"Start."}}`)
	write(`{"type":"ai-title","aiTitle":"A generated title"}`)

	// Filler the summary must never look at. Every line claims a timestamp and a title that would be wrong to report.
	line := `{"type":"assistant","uuid":"mid","timestamp":"2026-09-09T00:00:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"` +
		strings.Repeat("filler ", 40) + `"}]}}`
	for written := 0; written < filler; written += len(line) + 1 {
		write(line)
	}

	write(`{"type":"custom-title","customTitle":"The last title wins"}`)
	write(`{"type":"assistant","uuid":"t1","timestamp":"2026-08-02T00:00:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"Done."}]}}`)
	write(`{"type":"mode","mode":"normal"}`)

	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func ids(sums []Summary) string {
	out := make([]string, 0, len(sums))
	for _, s := range sums {
		out = append(out, s.ID)
	}
	return strings.Join(out, ", ")
}
