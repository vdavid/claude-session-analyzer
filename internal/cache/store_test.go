package cache

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/report"
	"github.com/vdavid/claude-session-analyzer/internal/session"
)

// stockholm is a fixed zone rather than a loaded one, so the test needs no tzdata on the machine running it.
var stockholm = time.FixedZone("Europe/Stockholm", 2*60*60)

func testStore(t *testing.T) *Store {
	t.Helper()
	return OpenAt(filepath.Join(t.TempDir(), "cache"))
}

// sample is a digest and a detail worth storing, without going near a transcript. What the store owes a caller is that
// what comes back is what went in, and that anything doubtful reads as a miss.
func sample(loc session.Location, fingerprint, zone string) (Digest, Detail) {
	digest := Digest{
		Version:     Version,
		Fingerprint: fingerprint,
		Zone:        zone,
		Built:       time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC),
		Session:     report.Session{ID: loc.ID, ProjectSlug: loc.ProjectSlug},
		Totals:      report.Totals{Lanes: 2},
		Cells:       []Cell{{Kind: "thinking", Day: "2026-08-03", Seconds: 12.5, Rows: 3}},
	}
	detail := Detail{
		Version:     Version,
		Fingerprint: fingerprint,
		Zone:        zone,
		SessionID:   loc.ID,
		Lanes:       []DetailLane{{ID: loc.ID, Name: "lead", IsLead: true, Seconds: 25}},
		Cells:       []Cell{{Lane: loc.ID, Kind: "thinking", Seconds: 12.5}},
	}
	return digest, detail
}

func TestSaveAndLoadRoundTripBothTiers(t *testing.T) {
	s := testStore(t)
	loc := locate(t, copyFixtureRoot(t), laneSessionID)
	digest, detail := sample(loc, "abc123", "UTC")

	if err := s.Save(loc, digest, detail); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, ok := s.LoadDigest(loc, "abc123", time.UTC)
	if !ok {
		t.Fatal("digest missed straight after a save")
	}
	if got.Totals.Lanes != 2 || len(got.Cells) != 1 || got.Cells[0].Seconds != 12.5 {
		t.Errorf("digest came back as %+v", got)
	}

	gotDetail, ok := s.LoadDetail(loc, "abc123", time.UTC)
	if !ok {
		t.Fatal("detail missed straight after a save")
	}
	if len(gotDetail.Lanes) != 1 || gotDetail.Lanes[0].Name != "lead" {
		t.Errorf("detail came back as %+v", gotDetail)
	}
}

// TestALoadMissesOnAnythingDoubtful covers the invalidation rules in one table. Every one of these is a case where
// answering from the file would be answering a different question than the caller asked.
func TestALoadMissesOnAnythingDoubtful(t *testing.T) {
	cases := []struct {
		name string
		// stored is what lands on disk, and asked is what the caller wants.
		storedVersion     int
		storedFingerprint string
		storedZone        string
		askFingerprint    string
		askZone           *time.Location
		// corrupt rewrites the digest file after the save.
		corrupt []byte
		// remove deletes the digest file after the save.
		remove  bool
		wantHit bool
	}{
		{
			name: "everything agrees", storedVersion: Version, storedFingerprint: "fp1", storedZone: "UTC",
			askFingerprint: "fp1", askZone: time.UTC, wantHit: true,
		},
		{
			name: "the derivation moved on", storedVersion: Version + 1, storedFingerprint: "fp1", storedZone: "UTC",
			askFingerprint: "fp1", askZone: time.UTC,
		},
		{
			name: "the session grew", storedVersion: Version, storedFingerprint: "fp1", storedZone: "UTC",
			askFingerprint: "fp2", askZone: time.UTC,
		},
		{
			name: "the days are cut somewhere else", storedVersion: Version, storedFingerprint: "fp1", storedZone: "UTC",
			askFingerprint: "fp1", askZone: stockholm,
		},
		{
			name: "the file is half written", storedVersion: Version, storedFingerprint: "fp1", storedZone: "UTC",
			askFingerprint: "fp1", askZone: time.UTC, corrupt: []byte(`{"version": 1, "cel`),
		},
		{
			name: "the file holds nothing", storedVersion: Version, storedFingerprint: "fp1", storedZone: "UTC",
			askFingerprint: "fp1", askZone: time.UTC, corrupt: []byte{},
		},
		{
			name: "nothing was ever cached", storedVersion: Version, storedFingerprint: "fp1", storedZone: "UTC",
			askFingerprint: "fp1", askZone: time.UTC, remove: true,
		},
	}

	root := copyFixtureRoot(t)
	loc := locate(t, root, laneSessionID)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testStore(t)
			digest, detail := sample(loc, c.storedFingerprint, c.storedZone)
			digest.Version = c.storedVersion
			detail.Version = c.storedVersion
			if err := s.Save(loc, digest, detail); err != nil {
				t.Fatalf("save: %v", err)
			}

			path := filepath.Join(s.Dir(), loc.ProjectSlug, loc.ID+".digest.json")
			switch {
			case c.remove:
				if err := os.Remove(path); err != nil {
					t.Fatalf("remove the digest: %v", err)
				}
			case c.corrupt != nil:
				if err := os.WriteFile(path, c.corrupt, 0o644); err != nil {
					t.Fatalf("corrupt the digest: %v", err)
				}
			}

			if _, ok := s.LoadDigest(loc, c.askFingerprint, c.askZone); ok != c.wantHit {
				t.Errorf("hit = %v, want %v", ok, c.wantHit)
			}
		})
	}
}

// TestTheStoreKeepsOneFilePerSessionUnderItsProject holds the layout. One file per session is what lets several agents
// query at once without a lock: a per-project file would need read, modify, write.
func TestTheStoreKeepsOneFilePerSessionUnderItsProject(t *testing.T) {
	s := testStore(t)
	root := copyFixtureRoot(t)
	for _, id := range []string{laneSessionID, soloSessionID} {
		loc := locate(t, root, id)
		digest, detail := sample(loc, "fp", "UTC")
		if err := s.Save(loc, digest, detail); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}

	want := []string{
		filepath.Join(laneSlug, laneSessionID+".detail.json"),
		filepath.Join(laneSlug, laneSessionID+".digest.json"),
		filepath.Join(soloSlug, soloSessionID+".detail.json"),
		filepath.Join(soloSlug, soloSessionID+".digest.json"),
	}
	got := filesUnder(t, s.Dir())
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("cache holds\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestConcurrentSavesNeverLeaveATornFile is why a save writes a temp file and renames it. Several agents query the
// corpus at once, so two of them writing one session's digest is ordinary, and last writer wins is fine as long as
// what a reader sees is always a whole digest.
func TestConcurrentSavesNeverLeaveATornFile(t *testing.T) {
	s := testStore(t)
	loc := locate(t, copyFixtureRoot(t), laneSessionID)

	var wg sync.WaitGroup
	for w := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				digest, detail := sample(loc, "fp", "UTC")
				digest.Totals.Lanes = w
				if err := s.Save(loc, digest, detail); err != nil {
					t.Errorf("save: %v", err)
					return
				}
			}
		}()
	}

	stop := make(chan struct{})
	var reads sync.WaitGroup
	reads.Add(1)
	go func() {
		defer reads.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if d, ok := s.LoadDigest(loc, "fp", time.UTC); ok && len(d.Cells) != 1 {
				t.Errorf("read a digest holding %d cells, want the whole one", len(d.Cells))
				return
			}
		}
	}()

	wg.Wait()
	close(stop)
	reads.Wait()

	if got := filesUnder(t, s.Dir()); len(got) != 2 {
		t.Errorf("cache holds %v, want the two files and no temporary leftovers", got)
	}
}

func TestInfoReportsWhatIsCached(t *testing.T) {
	s := testStore(t)
	root := copyFixtureRoot(t)

	empty, err := s.Info()
	if err != nil {
		t.Fatalf("info on an empty cache: %v", err)
	}
	if empty.Sessions != 0 || empty.Bytes != 0 || !empty.Oldest.IsZero() {
		t.Errorf("empty cache reports %+v", empty)
	}

	older := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 8, 6, 8, 0, 0, 0, time.UTC)
	for id, built := range map[string]time.Time{laneSessionID: older, soloSessionID: newer} {
		loc := locate(t, root, id)
		digest, detail := sample(loc, "fp", "UTC")
		digest.Built = built
		if err := s.Save(loc, digest, detail); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}

	info, err := s.Info()
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.Sessions != 2 {
		t.Errorf("sessions = %d, want 2 (a session is a digest, not a file)", info.Sessions)
	}
	if info.Bytes <= 0 {
		t.Errorf("bytes = %d, want what the four files cost", info.Bytes)
	}
	if !info.Oldest.Equal(older) || !info.Newest.Equal(newer) {
		t.Errorf("built span = %s to %s, want %s to %s", info.Oldest, info.Newest, older, newer)
	}
}

func TestPruneDropsSessionsThatAreGoneFromDisk(t *testing.T) {
	s := testStore(t)
	root := copyFixtureRoot(t)
	for _, id := range []string{laneSessionID, soloSessionID} {
		loc := locate(t, root, id)
		digest, detail := sample(loc, "fp", "UTC")
		if err := s.Save(loc, digest, detail); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}

	dropped, err := s.Prune(map[string]bool{laneSessionID: true})
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if dropped != 1 {
		t.Errorf("dropped = %d, want the one session that's gone", dropped)
	}

	loc := locate(t, root, laneSessionID)
	if _, ok := s.LoadDigest(loc, "fp", time.UTC); !ok {
		t.Error("prune dropped the session that's still on disk")
	}
	want := []string{
		filepath.Join(laneSlug, laneSessionID+".detail.json"),
		filepath.Join(laneSlug, laneSessionID+".digest.json"),
	}
	if got := filesUnder(t, s.Dir()); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("cache holds %v, want %v", got, want)
	}
}

func TestClearEmptiesTheCache(t *testing.T) {
	s := testStore(t)
	loc := locate(t, copyFixtureRoot(t), laneSessionID)
	digest, detail := sample(loc, "fp", "UTC")
	if err := s.Save(loc, digest, detail); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := s.Clear(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, ok := s.LoadDigest(loc, "fp", time.UTC); ok {
		t.Error("a digest survived a clear")
	}
	if got := filesUnder(t, s.Dir()); len(got) != 0 {
		t.Errorf("cache holds %v after a clear", got)
	}

	// Clearing twice, and saving after a clear, both have to work: the directory is gone at that point.
	if err := s.Clear(); err != nil {
		t.Fatalf("clear an already empty cache: %v", err)
	}
	if err := s.Save(loc, digest, detail); err != nil {
		t.Fatalf("save after a clear: %v", err)
	}
}

// TestOpenKeepsTheCacheOutOfTheTranscriptRoot covers where the cache lands. Writing inside `~/.claude/projects` would
// put generated files among Claude Code's own irreplaceable data.
func TestOpenKeepsTheCacheOutOfTheTranscriptRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))

	first, err := Open(filepath.Join(home, "claude", "projects"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !strings.HasPrefix(first.Dir(), filepath.Join(home, "cache", "claude-session-analyzer")) {
		t.Errorf("cache dir = %s, want it under XDG_CACHE_HOME", first.Dir())
	}

	again, err := Open(filepath.Join(home, "claude", "projects"))
	if err != nil {
		t.Fatalf("open again: %v", err)
	}
	if again.Dir() != first.Dir() {
		t.Errorf("one root opened two directories: %s and %s", first.Dir(), again.Dir())
	}

	other, err := Open(filepath.Join(home, "elsewhere", "projects"))
	if err != nil {
		t.Fatalf("open another root: %v", err)
	}
	if other.Dir() == first.Dir() {
		t.Errorf("two roots share %s, so one corpus would answer for the other", other.Dir())
	}
}

func TestOpenRefusesToCacheInsideTheTranscriptRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))

	if _, err := Open(root); err == nil {
		t.Error("opened a cache inside the transcript root, want a refusal")
	}
}

// filesUnder lists every file in the cache, relative to its directory, sorted.
func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		switch {
		case err != nil && os.IsNotExist(err) && path == dir:
			return filepath.SkipAll
		case err != nil:
			return err
		case d.IsDir():
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		out = append(out, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walk the cache: %v", err)
	}
	return out
}
