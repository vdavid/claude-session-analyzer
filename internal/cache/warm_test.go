package cache

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/session"
)

func TestCorpusParsesOnceAndServesTheSecondCallFromCache(t *testing.T) {
	s := testStore(t)
	root := copyFixtureRoot(t)

	first, err := s.Corpus(root, CorpusOptions{})
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if first.Parsed != 2 || first.Hits != 0 {
		t.Fatalf("first pass parsed %d and hit %d, want both sessions parsed", first.Parsed, first.Hits)
	}
	if len(first.Digests) != 2 {
		t.Fatalf("first pass gave %d digests, want 2", len(first.Digests))
	}

	second, err := s.Corpus(root, CorpusOptions{})
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if second.Hits != 2 || second.Parsed != 0 {
		t.Errorf("second pass hit %d and parsed %d, want both served from cache", second.Hits, second.Parsed)
	}

	// A cached answer has to be the answer, not an approximation of it.
	for i, want := range digestsByID(first.Digests) {
		got := digestsByID(second.Digests)[i]
		if got.Totals.WallClockSeconds != want.Totals.WallClockSeconds || len(got.Cells) != len(want.Cells) {
			t.Errorf("session %s came back as %+v, want %+v", i, got.Totals, want.Totals)
		}
	}
}

// TestCorpusReParsesASessionWhoseLaneMoved is the fingerprint doing its job through the whole path: a subagent
// appending to its own transcript has to invalidate the session even though the lead never moved.
func TestCorpusReParsesASessionWhoseLaneMoved(t *testing.T) {
	s := testStore(t)
	root := copyFixtureRoot(t)

	if _, err := s.Corpus(root, CorpusOptions{}); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	touch(t, laneFile(root))

	again, err := s.Corpus(root, CorpusOptions{})
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if again.Parsed != 1 || again.Hits != 1 {
		t.Errorf("parsed %d and hit %d, want only the touched session re-parsed", again.Parsed, again.Hits)
	}
}

func TestCorpusHonoursInclude(t *testing.T) {
	s := testStore(t)
	root := copyFixtureRoot(t)

	got, err := s.Corpus(root, CorpusOptions{
		Include: func(sum session.Summary) bool { return sum.ID == soloSessionID },
	})
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}
	if len(got.Digests) != 1 || got.Digests[0].Session.ID != soloSessionID {
		t.Fatalf("digests = %+v, want only the solo session", got.Digests)
	}
	if _, ok := s.LoadDigest(locate(t, root, laneSessionID), fingerprintOf(t, locate(t, root, laneSessionID)), time.UTC); ok {
		t.Error("a session Include left out was cached anyway")
	}
}

func TestCorpusLoadsTierTwoOnlyWhenAsked(t *testing.T) {
	s := testStore(t)
	root := copyFixtureRoot(t)

	without, err := s.Corpus(root, CorpusOptions{})
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}
	if len(without.Details) != 0 {
		t.Errorf("details = %+v, want none without Detail", without.Details)
	}

	with, err := s.Corpus(root, CorpusOptions{Detail: true})
	if err != nil {
		t.Fatalf("corpus with detail: %v", err)
	}
	if with.Hits != 2 {
		t.Errorf("hits = %d, want tier two served from the same files the first pass wrote", with.Hits)
	}
	detail, ok := with.Details[laneSessionID]
	if !ok {
		t.Fatalf("details = %+v, want one per session", with.Details)
	}
	if len(detail.Lanes) != 2 {
		t.Errorf("lanes = %+v, want the lead and its worker", detail.Lanes)
	}
}

// TestCorpusWithNoCacheStoresNothing covers the escape hatch: a person who suspects the cache wants an answer derived
// from the transcripts, and doesn't want the suspect answer written back either.
func TestCorpusWithNoCacheStoresNothing(t *testing.T) {
	s := testStore(t)
	root := copyFixtureRoot(t)

	if _, err := s.Corpus(root, CorpusOptions{}); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	before := filesUnder(t, s.Dir())

	got, err := s.Corpus(root, CorpusOptions{NoCache: true})
	if err != nil {
		t.Fatalf("no-cache pass: %v", err)
	}
	if got.Parsed != 2 || got.Hits != 0 {
		t.Errorf("parsed %d and hit %d, want everything re-derived", got.Parsed, got.Hits)
	}
	if after := filesUnder(t, s.Dir()); len(after) != len(before) {
		t.Errorf("cache holds %v, want the %d files it held before", after, len(before))
	}
}

func TestCorpusReportsProgressOnce(t *testing.T) {
	s := testStore(t)
	root := copyFixtureRoot(t)

	var mu sync.Mutex
	var seen [][2]int
	_, err := s.Corpus(root, CorpusOptions{
		Progress: func(done, total int) {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, [2]int{done, total})
		},
	})
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}

	want := [][2]int{{1, 2}, {2, 2}}
	if len(seen) != len(want) {
		t.Fatalf("progress = %v, want one call per session", seen)
	}
	for i, w := range want {
		if seen[i] != w {
			t.Errorf("progress %d = %v, want %v (counted in one place, so it never repeats a number)", i, seen[i], w)
		}
	}
}

// TestCorpusKeepsGoingPastASessionItCannotRead holds the run against one broken session. A corpus scan that gives up
// on the first unreadable transcript is a scan nobody can use.
func TestCorpusKeepsGoingPastASessionItCannotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a mode 000 file, so there's nothing to fail on")
	}
	s := testStore(t)
	root := copyFixtureRoot(t)
	if err := os.Chmod(leadFile(root), 0o000); err != nil {
		t.Fatalf("make the lead unreadable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(leadFile(root), 0o644) })

	got, err := s.Corpus(root, CorpusOptions{})
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}
	if got.Failed != 1 || len(got.Errors) != 1 {
		t.Errorf("failed = %d with %v, want the one unreadable session counted", got.Failed, got.Errors)
	}
	if len(got.Digests) != 1 || got.Digests[0].Session.ID != soloSessionID {
		t.Errorf("digests = %+v, want the readable session's", got.Digests)
	}
}

// TestCorpusDoesNotCacheAParseOfAMovingTranscript is the one way this cache can go permanently wrong: pinning a half
// read parse to a fingerprint that stays valid means every later run serves the wrong answer and never notices. The
// answer is still returned, it's the storing that's skipped.
func TestCorpusDoesNotCacheAParseOfAMovingTranscript(t *testing.T) {
	s := testStore(t)
	root := copyFixtureRoot(t)

	const record = `{"type": "user", "timestamp": "2026-08-03T09:05:00.000Z", "message": {"role": "user", "content": "And again."}}`
	afterParse = func(loc session.Location) { appendLine(t, leadFile(root), record) }
	t.Cleanup(func() { afterParse = nil })

	got, err := s.Corpus(root, CorpusOptions{
		Include: func(sum session.Summary) bool { return sum.ID == laneSessionID },
	})
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}
	if len(got.Digests) != 1 {
		t.Fatalf("digests = %+v, want the session answered anyway", got.Digests)
	}
	if files := filesUnder(t, s.Dir()); len(files) != 0 {
		t.Errorf("cache holds %v, want nothing stored for a session that moved under the parse", files)
	}
}

func digestsByID(digests []Digest) map[string]Digest {
	out := make(map[string]Digest, len(digests))
	for _, d := range digests {
		out[d.Session.ID] = d
	}
	return out
}
