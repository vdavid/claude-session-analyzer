package session

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// TestRealSession parses a transcript that's actually on this machine, which is the only way to catch format drift
// that hand-written fixtures can't. It skips unless you point it at one:
//
//	CSA_REAL_SESSION_ID=<id or unique prefix> go test ./internal/session -run RealSession -v
//
// Set CSA_REAL_ROOT to read from somewhere other than the default transcript root.
func TestRealSession(t *testing.T) {
	id := os.Getenv("CSA_REAL_SESSION_ID")
	if id == "" {
		t.Skip("set CSA_REAL_SESSION_ID to check the parser against a transcript on this machine")
	}

	root := os.Getenv("CSA_REAL_ROOT")
	if root == "" {
		var err error
		if root, err = DefaultRoot(); err != nil {
			t.Fatalf("default root: %v", err)
		}
	}

	loc, err := Find(root, id)
	if err != nil {
		t.Fatalf("find %q: %v", id, err)
	}

	var after runtime.MemStats
	start := time.Now()

	s, err := Load(loc, transcript.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	elapsed := time.Since(start)
	runtime.GC()
	runtime.ReadMemStats(&after)

	t.Logf("session %s (%s)", s.ID, s.ProjectSlug)
	t.Logf("title %q, project %s", s.Title, s.ProjectPath)
	t.Logf("%d lanes parsed in %s, %d MB of live heap after", len(s.Lanes), elapsed.Round(time.Millisecond),
		after.HeapAlloc/(1<<20))

	total := transcript.Stats{SkippedTypes: map[string]int{}}
	for _, lane := range s.Lanes {
		st := lane.Stats
		if st.Decoded+st.Skipped+st.Blank+st.Malformed != st.Lines {
			t.Errorf("lane %s: %+v doesn't account for every line", lane.Name, st)
		}
		if st.Malformed != 0 {
			t.Errorf("lane %s: %d malformed lines, first at line %d: %v",
				lane.Name, st.Malformed, st.FirstMalformedLine, st.FirstMalformedErr)
		}
		total.Lines += st.Lines
		total.Decoded += st.Decoded
		total.Skipped += st.Skipped
		total.Blank += st.Blank
		total.Malformed += st.Malformed
		if st.LongestLine > total.LongestLine {
			total.LongestLine = st.LongestLine
		}
		for k, v := range st.SkippedTypes {
			total.SkippedTypes[k] += v
		}
	}

	t.Logf("%d lines: %d decoded, %d skipped, %d blank, %d malformed. Longest line %d bytes.",
		total.Lines, total.Decoded, total.Skipped, total.Blank, total.Malformed, total.LongestLine)

	kinds := make([]string, 0, len(total.SkippedTypes))
	for k := range total.SkippedTypes {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return total.SkippedTypes[kinds[i]] > total.SkippedTypes[kinds[j]] })
	for _, k := range kinds {
		t.Logf("  skipped %-24s %d", k, total.SkippedTypes[k])
	}

	for _, lane := range s.Lanes {
		var first, last time.Time
		for _, rec := range lane.Records {
			if rec.Timestamp.IsZero() {
				continue
			}
			if first.IsZero() {
				first = rec.Timestamp
			}
			last = rec.Timestamp
		}
		t.Logf("  lane %-24s %-16s %6d records  %s → %s", lane.Name, lane.Meta.Model, len(lane.Records),
			first.Format(time.RFC3339), last.Format(time.RFC3339))
	}
}

// TestRealCorpusSweep parses every transcript on this machine, which is how format drift gets caught early: a new
// record type shows up as a skip, and anything the reader can't decode shows up as a malformed line. It skips unless
// you ask for it, and it takes about half a minute over a few thousand transcripts:
//
//	CSA_SWEEP=1 go test ./internal/session -run CorpusSweep -v -timeout 20m
func TestRealCorpusSweep(t *testing.T) {
	if os.Getenv("CSA_SWEEP") == "" {
		t.Skip("set CSA_SWEEP=1 to parse every transcript on this machine")
	}

	root := os.Getenv("CSA_REAL_ROOT")
	if root == "" {
		var err error
		if root, err = DefaultRoot(); err != nil {
			t.Fatalf("default root: %v", err)
		}
	}

	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			t.Logf("walk %s: %v", path, err)
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	total := transcript.Stats{SkippedTypes: map[string]int{}}
	start := time.Now()
	for _, path := range paths {
		lane, err := loadLane(path, transcript.Options{})
		if err != nil {
			t.Errorf("load %s: %v", path, err)
			continue
		}
		st := lane.Stats
		if st.Decoded+st.Skipped+st.Blank+st.Malformed != st.Lines {
			t.Errorf("%s: %+v doesn't account for every line", path, st)
		}
		if st.Malformed != 0 {
			t.Errorf("%s: %d malformed lines, first at line %d: %v",
				path, st.Malformed, st.FirstMalformedLine, st.FirstMalformedErr)
		}
		total.Lines += st.Lines
		total.Decoded += st.Decoded
		total.Skipped += st.Skipped
		total.Blank += st.Blank
		total.Malformed += st.Malformed
		if st.LongestLine > total.LongestLine {
			total.LongestLine = st.LongestLine
		}
		for k, v := range st.SkippedTypes {
			total.SkippedTypes[k] += v
		}
	}

	t.Logf("%d transcripts in %s", len(paths), time.Since(start).Round(time.Millisecond))
	t.Logf("%d lines: %d decoded, %d skipped, %d blank, %d malformed. Longest line %d bytes.",
		total.Lines, total.Decoded, total.Skipped, total.Blank, total.Malformed, total.LongestLine)

	kinds := make([]string, 0, len(total.SkippedTypes))
	for k := range total.SkippedTypes {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return total.SkippedTypes[kinds[i]] > total.SkippedTypes[kinds[j]] })
	for _, k := range kinds {
		t.Logf("  skipped %-28s %d", k, total.SkippedTypes[k])
	}
}
