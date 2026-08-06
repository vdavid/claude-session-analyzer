package timeline

import (
	"strings"
	"testing"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/session"
)

// teammateMessage is how a message from another agent arrives in a lane: an envelope the harness wraps around the
// text, naming who sent it. A lead sees a preamble line first; a subagent gets the envelope on its own.
func teammateMessage(from, body string) string {
	return `<teammate-message teammate_id="` + from + `" color="green">` + "\n" + body + "\n</teammate-message>"
}

func relayedTeammateMessage(from, body string) string {
	return "Another Claude session sent a message:\n" + teammateMessage(from, body)
}

// TestWaitNamesWhatEndedIt covers the column that makes the timeline worth reading: a lead's idle stretch says who it
// was idle on.
func TestWaitNamesWhatEndedIt(t *testing.T) {
	cases := []struct {
		name string
		end  string
		want string
	}{
		{"a person typing", "Could you also check the other lane?", "waiting for the next prompt"},
		{"a teammate replying", relayedTeammateMessage("m1-engine", "M1 is done."), "waiting for teammate m1-engine"},
		{"a teammate writing to a subagent", teammateMessage("team-lead", "Stop editing."), "waiting for teammate team-lead"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lane := newLane("lead", true).
				add(0, promptRec("go")).
				add(5, assistantRec(textBlock("done"))).
				add(6, systemRec("stop_hook_summary")).
				add(3600, promptRec(c.end)).
				add(3605, assistantRec(textBlock("on it"))).
				done()

			rows := Derive(sessionOf(lane), Options{}).Rows
			wait := rowOfKind(t, rows, KindWaiting)
			if !strings.HasPrefix(wait.Info, c.want) {
				t.Errorf("the wait says %q, want it to start with %q", wait.Info, c.want)
			}
			if got := wait.Duration(); got != 3595*time.Second {
				t.Errorf("the wait lasted %s, want the whole stretch from the last block to the prompt", got)
			}
		})
	}
}

// TestWaitEndsWhenQueuedInputArrives covers a background task's notification, which lands in the queue rather than as
// a prompt. It timestamps when the input actually arrived, so the wait ends there and the agent's thinking starts
// there.
func TestWaitEndsWhenQueuedInputArrives(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("started it in the background"))).
		add(6, systemRec("stop_hook_summary")).
		add(900, queueRec("enqueue", "<task-notification>\n<task-id>b19akwfoq</task-id>\n</task-notification>")).
		add(930, assistantRec(thinkingBlock(""))).
		add(935, assistantRec(textBlock("the task finished"))).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	requireKinds(t, rows, KindWriting, KindWaiting, KindThinking, KindWriting)
	if got := rows[1].Duration(); got != 895*time.Second {
		t.Errorf("the wait lasted %s, want it to end when the notification arrived", got)
	}
	if !strings.HasPrefix(rows[1].Info, "waiting for a background task") {
		t.Errorf("the wait says %q, want it to name the background task", rows[1].Info)
	}
	if got := rows[2].Duration(); got != 30*time.Second {
		t.Errorf("thinking lasted %s, want the 30s from the notification to the block", got)
	}
}

// TestSilentResumeIsNotThinking covers the case that would otherwise be invisible and badly wrong: the turn ended, and
// the next thing in the lane is the agent working again, hours later, with nothing to say what woke it. Calling that
// stretch thinking would report hours of reasoning that never happened.
func TestSilentResumeIsNotThinking(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("done"))).
		add(6, systemRec("turn_duration")).
		add(50000, assistantRec(thinkingBlock(""))).
		add(50010, assistantRec(textBlock("carrying on"))).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	requireKinds(t, rows, KindWriting, KindWaiting, KindThinking, KindWriting)
	if got := rows[1].Duration(); got != 49995*time.Second {
		t.Errorf("the wait lasted %s, want the whole stretch to the resumed block", got)
	}
	if rows[2].Duration() != 0 {
		t.Errorf("with nothing saying when the agent woke, the thinking row can't claim time: %s", rowSummary(rows[2]))
	}
	if !strings.Contains(rows[1].Info, "idle after the turn ended") {
		t.Errorf("the wait says %q, want it to say the turn had ended", rows[1].Info)
	}
}

// TestQueuedInputWhileWorkingIsNotAWait covers input arriving mid-turn. The agent was busy, so nothing about that is
// idle time.
func TestQueuedInputWhileWorkingIsNotAWait(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(toolUseBlock("t1", "Bash", "command", "cargo build"))).
		add(10, queueRec("enqueue", "one more thing")).
		add(60, toolResultRec("t1", "ok")).
		add(65, assistantRec(textBlock("built"))).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	requireKinds(t, rows, KindToolCall, KindToolExecution, KindWriting)
}

// TestLeadWaitNamesLiveTeammates covers Decision 8: while the lead sat idle, which teammates were alive. It's the
// single most useful thing the file says about a multi-agent session.
func TestLeadWaitNamesLiveTeammates(t *testing.T) {
	lead := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("spawning"))).
		add(6, systemRec("stop_hook_summary")).
		add(3600, promptRec(relayedTeammateMessage("m1-engine", "done"))).
		add(3605, assistantRec(textBlock("thanks"))).
		done()
	engine := newLane("m1-engine", false).
		add(10, promptRec("your task")).
		add(3000, assistantRec(textBlock("done"))).
		done()
	// This one starts after the wait ended, so it wasn't around for it.
	late := newLane("m2-timeline", false).
		add(4000, promptRec("your task")).
		add(4100, assistantRec(textBlock("done"))).
		done()

	rows := Derive(sessionOf(lead, engine, late), Options{}).Rows
	wait := rowOfKind(t, rows, KindWaiting)

	if !strings.Contains(wait.Info, "1 teammate alive: m1-engine") {
		t.Errorf("the wait says %q, want it to name the teammate that was alive", wait.Info)
	}
	if strings.Contains(wait.Info, "m2-timeline") {
		t.Errorf("the wait names a teammate that hadn't started yet: %q", wait.Info)
	}
}

// TestLeadWaitCountsCrowdedSessions covers a session with more teammates than a row can list. One session on the
// machine this was built against runs 977 workflow lanes, and a row per lane per wait would be unreadable.
func TestLeadWaitCountsCrowdedSessions(t *testing.T) {
	lanes := []*session.Lane{}
	lead := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("spawning"))).
		add(6, systemRec("stop_hook_summary")).
		add(3600, promptRec("carry on")).
		add(3605, assistantRec(textBlock("ok"))).
		done()
	lanes = append(lanes, lead)
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		lanes = append(lanes, newLane(name, false).
			add(10, promptRec("task")).
			add(3000, assistantRec(textBlock("done"))).
			done())
	}

	rows := Derive(sessionOf(lanes...), Options{}).Rows
	wait := rowOfKind(t, rows, KindWaiting)

	if !strings.Contains(wait.Info, "7 teammates alive") {
		t.Errorf("the wait says %q, want the count of teammates", wait.Info)
	}
	if !strings.Contains(wait.Info, "and 2 more") {
		t.Errorf("the wait says %q, want the tail of the list summarised", wait.Info)
	}
}

// TestSubagentWaitsAreNotAnnotated covers keeping the teammate list to the lead. A session with a thousand lanes would
// otherwise spend most of its output saying who else was running.
func TestSubagentWaitsAreNotAnnotated(t *testing.T) {
	lead := newLane("lead", true).
		add(0, promptRec("go")).
		add(4000, assistantRec(textBlock("done"))).
		done()
	worker := newLane("worker", false).
		add(10, promptRec("your task")).
		add(20, assistantRec(textBlock("asking"))).
		add(3000, promptRec(teammateMessage("team-lead", "carry on"))).
		add(3010, assistantRec(textBlock("ok"))).
		done()

	rows := Derive(sessionOf(lead, worker), Options{}).Rows
	wait := rowOfKind(t, rows, KindWaiting)

	if strings.Contains(wait.Info, "alive") {
		t.Errorf("a subagent's wait shouldn't list the session's other lanes: %q", wait.Info)
	}
}

// TestSpawnRowNamesTheTeammate covers the link from a spawn call to the lane it created, which the result carries
// directly rather than leaving us to match on names.
func TestSpawnRowNamesTheTeammate(t *testing.T) {
	lead := newLane("lead", true).
		add(0, promptRec("go")).
		add(4, assistantRec(toolUseBlock("t1", "Agent", "description", "Build the engine"))).
		add(5, withResultPayload(toolResultRec("t1", "spawned"),
			`{"status":"teammate_spawned","agent_id":"m1-engine@session-532ac591","name":"m1-engine",
			  "model":"claude-opus-5","color":"blue"}`)).
		add(600, assistantRec(textBlock("waiting on it"))).
		done()

	rows := Derive(sessionOf(lead), Options{}).Rows
	exec := rowOfKind(t, rows, KindToolExecution)

	if !strings.Contains(exec.Info, "spawned teammate m1-engine") {
		t.Errorf("the spawn row says %q, want it to name the teammate", exec.Info)
	}
	if !strings.Contains(exec.Info, "claude-opus-5") {
		t.Errorf("the spawn row says %q, want it to name the model", exec.Info)
	}
}

func rowOfKind(t *testing.T, rows []Row, kind Kind) Row {
	t.Helper()
	for _, r := range rows {
		if r.Kind == kind {
			return r
		}
	}
	t.Fatalf("no %s row:\n%s", kind, dump(rows))
	return Row{}
}
