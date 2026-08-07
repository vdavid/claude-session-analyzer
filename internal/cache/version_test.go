package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

// goldenPath is the derivation's committed output. It's the whole derivation held to a file, which makes it the honest
// answer to "did the rules change".
const goldenPath = "../timeline/testdata/golden/timeline.csv"

// goldenFingerprint is what that file hashed to when Version was last set. The two move together, and this test is the
// thing that makes them.
const goldenFingerprint = "c12fd8fb405e65aa2f4cf15aeebfd7ee6963f37ea861dad02dd374eb77bfce14"

// TestTheDigestVersionMovesWithTheDerivation is the guard on the one way this cache can be invisibly wrong.
//
// A digest is an answer, and an answer is only valid for the rules that produced it. Change a rule in
// `internal/timeline` without bumping Version and every cached digest on the machine keeps being served, silently,
// under the old rules. Nothing downstream can tell. So the golden CSV, which already holds the whole derivation, is
// hashed here: a derivation change moves the hash, this test fails, and the failure says to bump Version.
func TestTheDigestVersionMovesWithTheDerivation(t *testing.T) {
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read the golden timeline: %v", err)
	}

	sum := sha256.Sum256(golden)
	got := hex.EncodeToString(sum[:])
	if got == goldenFingerprint {
		return
	}

	t.Fatalf(`The derivation's output changed, so every cached digest on disk is answering under the old rules.

Bump Version in digest.go (it's %d now), and set goldenFingerprint to:

	%s

If the golden itself was rewritten on purpose (go test ./internal/timeline -update), that's exactly the case this
catches: the numbers a digest holds came out of those rules.`, Version, got)
}
