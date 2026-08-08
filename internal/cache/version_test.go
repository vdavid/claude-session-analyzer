package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

// goldenPath is the derivation's committed output. It's the whole derivation held to a file, which makes it the honest
// answer to "did the rules change".
const goldenPath = "../timeline/testdata/golden/timeline.csv"

// goldenFingerprint is what that file hashed to when Version was last set.
const goldenFingerprint = "c12fd8fb405e65aa2f4cf15aeebfd7ee6963f37ea861dad02dd374eb77bfce14"

// classificationFingerprint is what `timeline.ClassificationFingerprint` hashed to when Version was last set. It covers
// every class, its category, every group override, and what a representative call per class is identified as.
//
// It's here because the golden alone can't see a rule the fixture has no call for, which has now happened twice: a
// stored number changing meaning and the `lint` split off `build`, both invisible while every cached digest went stale.
const classificationFingerprint = "1a99c1e54b5eba6b6bb274282dba37c294789ad9e3417ae65035bad9731d8836"

// TestTheDigestVersionMovesWithTheDerivation is the guard on the one way this cache can be invisibly wrong.
//
// A digest is an answer, and an answer is only valid for the rules that produced it. Change a rule in
// `internal/timeline` without bumping Version and every cached digest on the machine keeps being served, silently,
// under the old rules. Nothing downstream can tell.
//
// So two fingerprints, because one rule change shows up in one of them and not the other:
//
//   - The golden CSV, which holds the whole derivation for one fixture session: the walk, the waits, the stalls, the
//     durations.
//   - The classification, which holds the class and category mapping whatever that fixture happens to contain. A class
//     no fixture call reaches is exactly the change the golden misses.
//
// The failure says which of the two moved, because they call for the same fix and mean different things.
func TestTheDigestVersionMovesWithTheDerivation(t *testing.T) {
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read the golden timeline: %v", err)
	}

	gotGolden := sum(string(golden))
	gotClassification := sum(timeline.ClassificationFingerprint())
	if gotGolden == goldenFingerprint && gotClassification == classificationFingerprint {
		return
	}

	moved, constants := "", ""
	if gotGolden != goldenFingerprint {
		moved += `
The derivation's OUTPUT changed: the golden CSV hashes differently, so the rows a digest was summed from moved. If the
golden was rewritten on purpose (go test ./internal/timeline -update), that's exactly the case this catches.
`
		constants += "\n\tgoldenFingerprint = \"" + gotGolden + "\""
	}
	if gotClassification != classificationFingerprint {
		moved += `
The CLASSIFICATION changed: a class, a category, a group override, or what a representative call is identified as. The
golden may not have moved at all, and every cached cell still carries the old class, group, and category.
`
		constants += "\n\tclassificationFingerprint = \"" + gotClassification + "\""
	}

	t.Fatalf(`Every cached digest on disk is now answering under the old rules.
%s
Bump Version in digest.go (it's %d now), and set:
%s

If a mapping moved on purpose, that's the change to record. A digest is an answer, and an answer under the old
definitions is invisibly wrong even when the rows behind it never moved.`, moved, Version, constants)
}

func sum(s string) string {
	digest := sha256.Sum256([]byte(s))
	return hex.EncodeToString(digest[:])
}
