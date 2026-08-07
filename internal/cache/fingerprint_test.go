package cache

import (
	"path/filepath"
	"testing"
)

// TestFingerprintCoversEveryFileTheSessionIsWrittenAcross is the one that matters. A subagent appends to its own
// transcript and the lead's mtime doesn't move, so a fingerprint over the lead alone reads a changed session as
// unchanged and the cache goes quietly stale.
func TestFingerprintCoversEveryFileTheSessionIsWrittenAcross(t *testing.T) {
	const record = `{"type": "assistant", "timestamp": "2026-08-03T09:01:00.000Z", "message": {"model": "claude-opus-5", "content": [{"type": "text", "text": "More."}]}}`

	cases := []struct {
		name       string
		change     func(t *testing.T, root string)
		wantChange bool
	}{
		{
			name:   "nothing changes",
			change: func(*testing.T, string) {},
		},
		{
			name:       "the lead grows",
			change:     func(t *testing.T, root string) { appendLine(t, leadFile(root), record) },
			wantChange: true,
		},
		{
			name:       "a lane grows",
			change:     func(t *testing.T, root string) { appendLine(t, laneFile(root), record) },
			wantChange: true,
		},
		{
			name:       "a lane is touched without growing",
			change:     func(t *testing.T, root string) { touch(t, laneFile(root)) },
			wantChange: true,
		},
		{
			name: "a lane grows without its mtime moving",
			change: func(t *testing.T, root string) {
				appendLine(t, laneFile(root), record)
				restoreTime(t, laneFile(root))
			},
			wantChange: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := copyFixtureRoot(t)
			loc := locate(t, root, laneSessionID)

			before := fingerprintOf(t, loc)
			c.change(t, root)
			after := fingerprintOf(t, loc)

			if changed := before != after; changed != c.wantChange {
				t.Errorf("fingerprint changed = %v (%s to %s), want %v", changed, before, after, c.wantChange)
			}
		})
	}
}

func TestFingerprintTellsTwoSessionsApart(t *testing.T) {
	root := copyFixtureRoot(t)
	lane := fingerprintOf(t, locate(t, root, laneSessionID))
	solo := fingerprintOf(t, locate(t, root, soloSessionID))
	if lane == solo {
		t.Errorf("both sessions fingerprint to %s", lane)
	}
}

// TestFingerprintDoesNotDependOnWhereTheRootSits holds the keying to paths relative to the transcript root. A corpus
// moved or mounted somewhere else is the same corpus, and re-parsing 3.8 GB over a changed prefix would be waste.
func TestFingerprintDoesNotDependOnWhereTheRootSits(t *testing.T) {
	first := fingerprintOf(t, locate(t, copyFixtureRoot(t), laneSessionID))
	second := fingerprintOf(t, locate(t, copyFixtureRoot(t), laneSessionID))
	if first != second {
		t.Errorf("two copies of one tree fingerprint to %s and %s", first, second)
	}
}

func TestFingerprintFailsWhenAFileIsGone(t *testing.T) {
	root := copyFixtureRoot(t)
	loc := locate(t, root, laneSessionID)
	loc.TranscriptPath = filepath.Join(root, laneSlug, "not-a-session.jsonl")

	if _, err := Fingerprint(loc); err == nil {
		t.Error("fingerprinting a missing transcript succeeded, want an error rather than a hash of nothing")
	}
}
