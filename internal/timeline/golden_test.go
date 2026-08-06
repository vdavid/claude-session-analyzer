package timeline

import (
	"bytes"
	"encoding/csv"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

var update = flag.Bool("update", false, "rewrite the golden files instead of comparing against them")

// goldenSession is a hand-built session under testdata carrying one of everything the derivation has a rule for: a
// spawn and the teammate it created, a wait ended by a teammate message, queued input, a compaction with the records
// it replays under older stamps, a response packed into one record with parallel calls, a timed-out wait loop, a
// six-hour stall, an honest 40-minute build, a lane with no metadata file, and a record type nothing decodes.
const goldenSession = "11111111-2222-3333-4444-555555555555"

// TestGoldenTimeline holds the whole derivation to a file, so changing a rule shows up as a reviewable diff rather
// than as a silent shift in what the tool claims. Rewrite it with `go test ./internal/timeline -update` and read the
// diff before committing it.
func TestGoldenTimeline(t *testing.T) {
	tl := deriveGolden(t)

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.WriteAll(tl.Records()); err != nil {
		t.Fatalf("write the CSV: %v", err)
	}
	w.Flush()

	compareGolden(t, "timeline.csv", buf.Bytes())
}

// TestGoldenTiling holds the fixture to the property the package rests on, lane by lane.
func TestGoldenTiling(t *testing.T) {
	tl := deriveGolden(t)

	for _, lane := range tl.Lanes {
		var rows []Row
		for _, r := range tl.Rows {
			if r.LaneID == lane.ID {
				rows = append(rows, r)
			}
		}
		t.Run(lane.Name, func(t *testing.T) {
			checkTiling(t, rows, lane.First, lane.Last)
		})
	}
}

func deriveGolden(t *testing.T) *Timeline {
	t.Helper()
	loc, err := session.Find(filepath.Join("testdata", "projects"), goldenSession)
	if err != nil {
		t.Fatalf("find the fixture session: %v", err)
	}
	s, err := session.Load(loc, transcript.Options{})
	if err != nil {
		t.Fatalf("load the fixture session: %v", err)
	}
	return Derive(s, Options{})
}

func compareGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)

	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("make the golden directory: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("rewrote %s", path)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s (run with -update to create it): %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is out of date. Run `go test ./internal/timeline -update` and read the diff.\n\ngot:\n%s",
			path, got)
	}
}
