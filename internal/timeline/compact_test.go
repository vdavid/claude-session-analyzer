package timeline

import (
	"strings"
	"testing"
	"time"
)

// TestCompactionIsItsOwnKind covers the harness compacting a context. It's real wall clock that belongs to neither the
// agent nor a tool, and it eats minutes invisibly.
//
// The records around it disagree about when it happened: the boundary is stamped when compaction finished, while the
// records replayed after it carry the stamps they had before. So the row is placed by measuring back from the boundary
// with the duration the boundary itself reports, and the long idle stretch before it stays a wait.
func TestCompactionIsItsOwnKind(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(100, assistantRec(textBlock("done"))).
		add(3000, compactRec(132016*time.Millisecond, 674475, 10198)).
		add(2999, promptRec("This session is being continued from a previous conversation.")).
		add(2868, promptRec("<command-name>/compact</command-name>")).
		add(3005, assistantRec(textBlock("carrying on"))).
		done()

	tl := Derive(sessionOf(lane), Options{})

	requireKinds(t, tl.Rows, KindWriting, KindWaiting, KindCompacting, KindWriting)
	checkTiling(t, tl.Rows, at(0), at(3005))

	compaction := tl.Rows[2]
	if got := compaction.Duration(); got != 132016*time.Millisecond {
		t.Errorf("the compaction lasted %s, want the 132.016s it reported", got)
	}
	if !compaction.From.Equal(at(3000).Add(-132016 * time.Millisecond)) {
		t.Errorf("the compaction starts at %s, want 132.016s before the boundary", offset(compaction.From))
	}
	if got := tl.Rows[1].Duration(); got != 2767984*time.Millisecond {
		t.Errorf("the wait before the compaction lasted %s, want the whole stretch from 100s to the compaction", got)
	}
	if !strings.Contains(compaction.Info, "674,475") || !strings.Contains(compaction.Info, "10,198") {
		t.Errorf("the row should say what it compacted, got %q", compaction.Info)
	}
}

// TestCompactionWithoutADuration covers an older boundary record that reports no duration. Measuring back from the
// boundary is then guesswork, so the row marks the moment and the time before it stays a wait.
func TestCompactionWithoutADuration(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(100, assistantRec(textBlock("done"))).
		add(3000, compactRec(0, 0, 0)).
		add(3005, assistantRec(textBlock("carrying on"))).
		done()

	tl := Derive(sessionOf(lane), Options{})

	requireKinds(t, tl.Rows, KindWriting, KindWaiting, KindCompacting, KindWriting)
	checkTiling(t, tl.Rows, at(0), at(3005))
	if got := tl.Rows[2].Duration(); got != 0 {
		t.Errorf("with no duration reported the row should claim no time, got %s", got)
	}
}

// TestCompactionCannotReachBackPastTheLane covers a compaction reporting more time than the lane has left before it,
// which would otherwise push the row's start behind work that already happened.
func TestCompactionCannotReachBackPastTheLane(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(100, assistantRec(textBlock("done"))).
		add(130, compactRec(132016*time.Millisecond, 500000, 9000)).
		add(140, assistantRec(textBlock("carrying on"))).
		done()

	tl := Derive(sessionOf(lane), Options{})

	requireKinds(t, tl.Rows, KindWriting, KindCompacting, KindWriting)
	checkTiling(t, tl.Rows, at(0), at(140))
	if got := tl.Rows[1].Duration(); got != 30*time.Second {
		t.Errorf("the compaction row should stop where the lane's last activity did, got %s", got)
	}
}
