package timeline

import "testing"

// TestThinkingUsesRealReasoningWhenItSurvives covers the rare transcript that kept the thinking text. Two of 5,471
// sampled blocks did, so it's worth reading rather than assuming.
func TestThinkingUsesRealReasoningWhenItSurvives(t *testing.T) {
	lane := newLane("agent", false).
		add(0, promptRec("go")).
		add(10, assistantRec(thinkingBlock("The config is stale, so check the loader before the parser."))).
		add(12, assistantRec(toolUseBlock("t1", "Read", "file_path", "/tmp/loader.go"))).
		add(13, toolResultRec("t1", "ok")).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	if got := rows[0].Info; got != "The config is stale, so check the loader before the parser." {
		t.Errorf("the thinking row says %q, want the reasoning the block carried", got)
	}
}

// TestThinkingBorrowsFromWhatFollows covers the usual case: the thinking text is empty, so the only honest label is
// what the agent did next, phrased so it reads as inference rather than as a quote.
func TestThinkingBorrowsFromWhatFollows(t *testing.T) {
	lane := newLane("agent", false).
		add(0, promptRec("go")).
		add(10, assistantRec(thinkingBlock(""))).
		add(12, assistantRec(toolUseBlock("t1", "Bash", "command", "pnpm check -q clippy"))).
		add(20, toolResultRec("t1", "ok")).
		add(25, assistantRec(thinkingBlock(""))).
		add(30, assistantRec(textBlock("Everything passed."))).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	requireKinds(t, rows, KindThinking, KindToolCall, KindToolExecution, KindThinking, KindWriting)
	if got := rows[0].Info; got != "before Bash (checker): pnpm check -q clippy" {
		t.Errorf("the first thinking row says %q, want the call it led to", got)
	}
	if got := rows[3].Info; got != "before writing: Everything passed." {
		t.Errorf("the second thinking row says %q, want the prose it led to", got)
	}
}

// TestThinkingWithNothingAfterIt covers a transcript that ends mid-thought, which a session read while it's running
// does often.
func TestThinkingWithNothingAfterIt(t *testing.T) {
	lane := newLane("agent", false).
		add(0, promptRec("go")).
		add(10, assistantRec(thinkingBlock(""))).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	if got := rows[0].Info; got != "nothing followed it in the transcript" {
		t.Errorf("the thinking row says %q, want it to say there was nothing to infer from", got)
	}
}

// TestConsecutiveThinkingSharesTheSubject covers a response that thought twice before acting.
func TestConsecutiveThinkingSharesTheSubject(t *testing.T) {
	lane := newLane("agent", false).
		add(0, promptRec("go")).
		add(10, assistantRec(thinkingBlock(""))).
		add(15, assistantRec(thinkingBlock(""))).
		add(20, assistantRec(toolUseBlock("t1", "Grep", "pattern", "panic"))).
		add(21, toolResultRec("t1", "none")).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	for _, i := range []int{0, 1} {
		if got := rows[i].Info; got != "before Grep (search): panic" {
			t.Errorf("thinking row %d says %q, want the call both of them led to", i, got)
		}
	}
}
