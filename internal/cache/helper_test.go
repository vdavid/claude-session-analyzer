package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/session"
)

// The two sessions in `testdata/projects`: one with a subagent lane, one with nothing but a lead.
const (
	laneSessionID = "c0000001-0000-4000-8000-000000000001"
	soloSessionID = "c0000002-0000-4000-8000-000000000002"
	laneSlug      = "-tmp-cache-one"
	soloSlug      = "-tmp-cache-two"
)

// fixtureTime is what every copied fixture file's mtime is set to, so a fingerprint depends on what a test changes
// rather than on when the tree was copied.
var fixtureTime = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

// copyFixtureRoot copies `testdata/projects` into a temp directory and pins every mtime, so a test can touch a
// transcript without dirtying the repo and without racing the clock.
func copyFixtureRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "projects")
	if err := os.CopyFS(root, os.DirFS(filepath.Join("testdata", "projects"))); err != nil {
		t.Fatalf("copy the fixture root: %v", err)
	}
	pinTimes(t, root)
	return root
}

// pinTimes sets every file under root to fixtureTime.
func pinTimes(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return os.Chtimes(path, fixtureTime, fixtureTime)
	})
	if err != nil {
		t.Fatalf("pin the fixture mtimes: %v", err)
	}
}

func locate(t *testing.T, root, id string) session.Location {
	t.Helper()
	loc, err := session.Find(root, id)
	if err != nil {
		t.Fatalf("find %s: %v", id, err)
	}
	return loc
}

func fingerprintOf(t *testing.T, loc session.Location) string {
	t.Helper()
	fp, err := Fingerprint(loc)
	if err != nil {
		t.Fatalf("fingerprint %s: %v", loc.ID, err)
	}
	return fp
}

// laneFile is the one subagent transcript the lane fixture holds.
func laneFile(root string) string {
	return filepath.Join(root, laneSlug, laneSessionID, "subagents", "agent-aworker-11112222.jsonl")
}

func leadFile(root string) string {
	return filepath.Join(root, laneSlug, laneSessionID+".jsonl")
}

// appendLine adds a record to a transcript, the way a running session does.
func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("append to %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

// touch moves a file's mtime a minute on, leaving its bytes alone.
func touch(t *testing.T, path string) {
	t.Helper()
	when := fixtureTime.Add(time.Minute)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("touch %s: %v", path, err)
	}
}

// restoreTime puts a file's mtime back where the fixture had it, which is how a test isolates a size-only change.
func restoreTime(t *testing.T, path string) {
	t.Helper()
	if err := os.Chtimes(path, fixtureTime, fixtureTime); err != nil {
		t.Fatalf("restore the mtime of %s: %v", path, err)
	}
}
