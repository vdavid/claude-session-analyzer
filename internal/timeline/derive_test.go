package timeline

import (
	"testing"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// checkTiling asserts the property the whole package rests on: a lane's rows cover its span exactly once. Every row
// starts where the last one ended, the first starts when the lane started, and the last ends when the lane ended. A
// batch of parallel tool calls is the one allowed exception, and each of those rows says so.
func checkTiling(t *testing.T, rows []Row, first, last time.Time) {
	t.Helper()
	if len(rows) == 0 {
		t.Fatal("no rows")
	}

	var chain []Row
	for _, r := range rows {
		if r.Duration() < 0 {
			t.Errorf("negative duration: %s", rowSummary(r))
		}
		if !r.Overlapped {
			chain = append(chain, r)
		}
	}

	if !chain[0].From.Equal(first) {
		t.Errorf("first row starts at %s, lane starts at %s", offset(chain[0].From), offset(first))
	}
	if !chain[len(chain)-1].Until.Equal(last) {
		t.Errorf("last row ends at %s, lane ends at %s", offset(chain[len(chain)-1].Until), offset(last))
	}
	for i := 1; i < len(chain); i++ {
		if !chain[i].From.Equal(chain[i-1].Until) {
			t.Errorf("gap or overlap between rows %d and %d:\n  %s\n  %s",
				i-1, i, rowSummary(chain[i-1]), rowSummary(chain[i]))
		}
	}
}

// TestDeriveWalksOneLane covers the ordinary shape: the agent thinks, calls a tool, waits for it, then writes back.
func TestDeriveWalksOneLane(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("do the thing")).
		add(12, assistantRec(thinkingBlock(""))).
		add(14, assistantRec(toolUseBlock("t1", "Bash", "command", "ls /tmp"))).
		add(20, toolResultRec("t1", "a\nb")).
		add(23, assistantRec(textBlock("Two files."))).
		done()

	tl := Derive(sessionOf(lane), Options{})

	requireKinds(t, tl.Rows, KindThinking, KindToolCall, KindToolExecution, KindWriting)
	checkTiling(t, tl.Rows, at(0), at(23))

	want := []time.Duration{12 * time.Second, 2 * time.Second, 6 * time.Second, 3 * time.Second}
	for i, w := range want {
		if got := tl.Rows[i].Duration(); got != w {
			t.Errorf("row %d lasted %s, want %s: %s", i, got, w, rowSummary(tl.Rows[i]))
		}
	}
	if tl.Rows[2].Tool != "Bash" {
		t.Errorf("the tool execution row names %q, want Bash", tl.Rows[2].Tool)
	}
}

// TestDeriveOneRecordManyBlocks covers older transcripts, which pack a whole response into one record. The blocks
// share a single timestamp, so only the first can honestly claim the span.
func TestDeriveOneRecordManyBlocks(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(30, assistantRec(
			thinkingBlock(""),
			textBlock("Reading both."),
			toolUseBlock("t1", "Read", "file_path", "/tmp/a"),
			toolUseBlock("t2", "Read", "file_path", "/tmp/b"),
		)).
		add(33, toolResultRec("t1", "a")).
		add(35, toolResultRec("t2", "b")).
		done()

	tl := Derive(sessionOf(lane), Options{})

	requireKinds(t, tl.Rows,
		KindThinking, KindWriting, KindToolCall, KindToolCall, KindToolExecution, KindToolExecution)
	checkTiling(t, tl.Rows, at(0), at(35))

	if got := tl.Rows[0].Duration(); got != 30*time.Second {
		t.Errorf("the first block of the record lasted %s, want the whole 30s span", got)
	}
	for _, i := range []int{1, 2, 3} {
		if tl.Rows[i].Duration() != 0 {
			t.Errorf("row %d should be zero-length, the record carries one timestamp for every block: %s",
				i, rowSummary(tl.Rows[i]))
		}
	}
	if !tl.Rows[4].Overlapped || tl.Rows[5].Overlapped {
		t.Errorf("the earlier of two parallel executions is the overlapping one:\n%s", dump(tl.Rows))
	}
}

// TestDeriveClampsBackwardsTimestamps covers write jitter, which stamps a record before the one it follows in 186 of
// the reference session's 15,831 records.
func TestDeriveClampsBackwardsTimestamps(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(10, assistantRec(textBlock("first"))).
		add(9.9, assistantRec(textBlock("stamped a hair earlier"))).
		add(12, assistantRec(textBlock("last"))).
		done()

	tl := Derive(sessionOf(lane), Options{})

	requireKinds(t, tl.Rows, KindWriting, KindWriting, KindWriting)
	checkTiling(t, tl.Rows, at(0), at(12))
	if got := tl.Rows[1].Duration(); got != 0 {
		t.Errorf("a backwards record should cost zero time, not %s", got)
	}
}

// TestDeriveWaitsBetweenTurns covers the gap that makes this tool worth building: the agent finished, and nothing
// happened until the next prompt landed.
func TestDeriveWaitsBetweenTurns(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("first")).
		add(5, assistantRec(textBlock("done"))).
		add(3605, promptRec("second")).
		add(3610, assistantRec(textBlock("done again"))).
		done()

	tl := Derive(sessionOf(lane), Options{})

	requireKinds(t, tl.Rows, KindWriting, KindWaiting, KindWriting)
	checkTiling(t, tl.Rows, at(0), at(3610))
	if got := tl.Rows[1].Duration(); got != time.Hour {
		t.Errorf("the wait lasted %s, want 1h", got)
	}
}

// TestDeriveIgnoresBookkeepingRecords covers the records that carry a timestamp but aren't work: hook attachments and
// turn summaries. Their time belongs to whatever row surrounds them.
//
// The queue is the exception, and only while the lane is idle: input arriving is a real moment, so it splits the wait
// into "nothing had come in yet" and "it had come in and the lane hadn't picked it up".
func TestDeriveIgnoresBookkeepingRecords(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(2, attachmentRec("hook_success")).
		add(8, assistantRec(textBlock("done"))).
		add(9, systemRec("turn_duration")).
		add(60, queueRec("enqueue", "next thing")).
		add(70, promptRec("next thing")).
		add(75, assistantRec(textBlock("ok"))).
		done()

	tl := Derive(sessionOf(lane), Options{})

	requireKinds(t, tl.Rows, KindWriting, KindWaiting, KindWaiting, KindWriting)
	checkTiling(t, tl.Rows, at(0), at(75))
	if got := tl.Rows[0].Duration(); got != 8*time.Second {
		t.Errorf("the attachment should not have split the writing row, which lasted %s", got)
	}
	if got := tl.Rows[1].Duration(); got != 52*time.Second {
		t.Errorf("the first wait lasted %s, want it to end when the input was queued", got)
	}
}

// TestDeriveClosesAnIdleTail covers a lane whose last records are bookkeeping: the lane sat idle to the end, and the
// rows still have to reach it.
func TestDeriveClosesAnIdleTail(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("done"))).
		add(400, systemRec("stop_hook_summary")).
		done()

	tl := Derive(sessionOf(lane), Options{})

	requireKinds(t, tl.Rows, KindWriting, KindWaiting)
	checkTiling(t, tl.Rows, at(0), at(400))
}

// TestDeriveMergesLanes covers a session with a subagent: rows come back in time order, and every lane reports its
// span.
func TestDeriveMergesLanes(t *testing.T) {
	lead := newLane("lead", true).
		add(0, promptRec("go")).
		add(4, assistantRec(toolUseBlock("t1", "Agent", "description", "spawn a helper"))).
		add(5, toolResultRec("t1", "spawned")).
		add(300, assistantRec(textBlock("all done"))).
		done()
	helper := newLane("helper", false).
		add(10, promptRec("your task")).
		add(20, assistantRec(textBlock("on it"))).
		done()

	tl := Derive(sessionOf(lead, helper), Options{})

	if len(tl.Lanes) != 2 {
		t.Fatalf("got %d lane spans, want 2", len(tl.Lanes))
	}
	if !tl.Lanes[1].First.Equal(at(10)) || !tl.Lanes[1].Last.Equal(at(20)) {
		t.Errorf("the helper span is %s to %s, want 10s to 20s", offset(tl.Lanes[1].First), offset(tl.Lanes[1].Last))
	}
	if !tl.First.Equal(at(0)) || !tl.Last.Equal(at(300)) {
		t.Errorf("the session runs %s to %s, want 0s to 5m00s", offset(tl.First), offset(tl.Last))
	}
	for i := 1; i < len(tl.Rows); i++ {
		if tl.Rows[i].From.Before(tl.Rows[i-1].From) {
			t.Fatalf("rows are out of order at %d:\n%s", i, dump(tl.Rows))
		}
	}
}

// TestDeriveSkipsAnEmptyLane covers a lane whose records carry no timestamps at all, which a session that died early
// leaves behind.
func TestDeriveSkipsAnEmptyLane(t *testing.T) {
	lead := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("done"))).
		done()

	tl := Derive(sessionOf(lead, newLane("stillborn", false).done()), Options{})

	if len(tl.Lanes) != 1 {
		t.Fatalf("a lane with no timestamps should not get a span, got %d", len(tl.Lanes))
	}
	checkTiling(t, tl.Rows, at(0), at(5))
}

// TestDeriveAbsorbsUnknownBlocks covers a block type nothing decodes. The one in the wild is `fallback`, which marks
// the harness switching models mid-response, and it isn't the agent doing anything. Its stretch belongs to the row
// that follows, and a record that produces no rows must not move the lane on without one.
func TestDeriveAbsorbsUnknownBlocks(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(10, assistantRec(transcript.Block{Type: "fallback"})).
		add(25, assistantRec(thinkingBlock(""))).
		add(26, assistantRec(textBlock("done"))).
		done()

	tl := Derive(sessionOf(lane), Options{})

	requireKinds(t, tl.Rows, KindThinking, KindWriting)
	checkTiling(t, tl.Rows, at(0), at(26))
	if got := tl.Rows[0].Duration(); got != 25*time.Second {
		t.Errorf("the thinking row lasted %s, want it to absorb the stretch the fallback block covered", got)
	}
}
