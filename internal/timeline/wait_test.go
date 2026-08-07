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

// taskNotification is how a background task reports in: the harness's own envelope, carrying the task's id and the
// tool-use id of the call that started it. 9,884 of the corpus's 12,076 notifications carry the tool-use id (verified
// 2026-08-07); the ones that don't are covered by the case below that leaves the id out.
func taskNotification(taskID, toolUseID string) string {
	out := "<task-notification>\n<task-id>" + taskID + "</task-id>\n"
	if toolUseID != "" {
		out += "<tool-use-id>" + toolUseID + "</tool-use-id>\n"
	}
	return out + "<status>completed</status>\n<summary>Background command finished</summary>\n</task-notification>"
}

// TestWaitNamesWhatEndedIt covers the column that makes the timeline worth reading: a lead's idle stretch says who it
// was idle on, in the kind as well as in the info.
func TestWaitNamesWhatEndedIt(t *testing.T) {
	cases := []struct {
		name string
		end  string
		kind Kind
		want string
	}{
		{"a person typing", "Could you also check the other lane?", KindWaitingForPerson,
			"waiting for the next prompt"},
		{"a teammate replying", relayedTeammateMessage("m1-engine", "The reader is done."), KindWaitingForTeammate,
			"waiting for teammate m1-engine"},
		{"a teammate writing to a subagent", teammateMessage("team-lead", "Stop editing."), KindWaitingForTeammate,
			"waiting for teammate team-lead"},
		// A notification arrives as a prompt as often as it arrives queued: 2,044 against 6,288 across the corpus
		// (verified 2026-08-06), so watching only the queue would misfile a third of them as a person.
		{"a background task reporting in", "<task-notification>\n<task-id>b19akwfoq</task-id>\n</task-notification>",
			KindWaitingForTask, "waiting for a background task"},
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
			wait := rowOfKind(t, rows, c.kind)
			if !strings.HasPrefix(wait.Info, c.want) {
				t.Errorf("the wait says %q, want it to start with %q", wait.Info, c.want)
			}
			if got := wait.Duration(); got != 3595*time.Second {
				t.Errorf("the wait lasted %s, want the whole stretch from the last block to the prompt", got)
			}
		})
	}
}

// TestTaskWaitAsksWhoStartedTheTask covers the correction that keeps a notification from claiming a wait it didn't
// earn. A notification says a task finished, not that the lane sat idle for it: on session 532ac591 the lead sat idle
// 25m30s and was woken by a 0.14 s poll loop the subagent `m1-honesty` had left in the background, while that subagent
// was alive and working the whole time. Read as a background task's wait, that says the app took 25 minutes to launch.
//
// So the task's owner decides. Another lane's task, that lane still running: the wait was on the teammate. Anything
// else, including a task the lane started itself and a task whose owner had already finished, stays a background task's.
func TestTaskWaitAsksWhoStartedTheTask(t *testing.T) {
	cases := []struct {
		name      string
		toolUseID string
		kind      Kind
		want      string
	}{
		{"a task the lane started itself", "t1", KindWaitingForTask, "waiting for a background task"},
		{"a live teammate's task", "w1", KindWaitingForTeammate, "waiting for teammate worker, via its background task"},
		// A teammate that starts a long build, reports back, and exits leaves the lead genuinely waiting on the build.
		{"a finished teammate's task", "e1", KindWaitingForTask, "waiting for a background task"},
		{"a task no lane claims", "toolu_nobody", KindWaitingForTask, "waiting for a background task"},
		{"a notification carrying no tool-use id", "", KindWaitingForTask, "waiting for a background task"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lead := newLane("lead", true).
				add(0, promptRec("bring the app up and build it")).
				add(5, assistantRec(toolUseBlock("t1", "Bash", "command", "cargo build --release"))).
				add(10, toolResultRec("t1", "moved to the background")).
				add(12, assistantRec(textBlock("left the build running"))).
				add(13, systemRec("turn_duration")).
				add(3600, queueRec("enqueue", taskNotification("b19akwfoq", c.toolUseID))).
				add(3605, assistantRec(textBlock("it finished"))).
				done()
			// Alive across the whole gap, and working: its own background task woke the lead.
			worker := newLane("worker", false).
				add(20, promptRec("your task")).
				add(30, assistantRec(toolUseBlock("w1", "Bash",
					"command", `until grep -q "Ready in" dev.log; do sleep 3; done`))).
				add(40, toolResultRec("w1", "moved to the background")).
				add(5000, assistantRec(textBlock("the app is up"))).
				done()
			// Gone long before the gap ended, so its task is the only thing the lead could have been waiting for.
			early := newLane("early", false).
				add(20, promptRec("your task")).
				add(30, assistantRec(toolUseBlock("e1", "Bash", "command", "cargo test --release"))).
				add(40, toolResultRec("e1", "moved to the background")).
				add(100, assistantRec(textBlock("tests are running"))).
				done()

			rows := Derive(sessionOf(lead, worker, early), Options{}).Rows
			wait := rowOfKindIn(t, rows, c.kind, "lead")

			if !strings.HasPrefix(wait.Info, c.want) {
				t.Errorf("the wait says %q, want it to start with %q", wait.Info, c.want)
			}
			// nameWaits still gets its say, and lands after the reclassification rather than on a stale kind.
			if !strings.Contains(wait.Info, "teammates alive: early, worker") {
				t.Errorf("the wait says %q, want it to still list who was alive", wait.Info)
			}
			if got := wait.Duration(); got != 3588*time.Second {
				t.Errorf("the wait lasted %s, want the stretch from the last block to the notification", got)
			}
			if !wait.Kind.IsWaiting() || !wait.Kind.IsGap() {
				t.Errorf("%s has to count as a wait and as a gap, or every total downstream drifts", wait.Kind)
			}

			var lane []Row
			for _, r := range rows {
				if r.LaneID == "lead" {
					lane = append(lane, r)
				}
			}
			checkTiling(t, lane, at(0), at(3605))
		})
	}
}

// TestWaitKindsAreSeparateBuckets covers the split itself: one session's waits land in different kinds, so a pie has a
// slice per thing waited on and a spreadsheet can group by the Activity column. Before the split, all four of these
// rows said "waiting" and the difference lived in a substring of the info.
func TestWaitKindsAreSeparateBuckets(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("spawned the worker and started the build"))).
		add(6, systemRec("turn_duration")).
		add(100, queueRec("enqueue", "<task-notification>\n<task-id>b19akwfoq</task-id>\n</task-notification>")).
		add(110, assistantRec(textBlock("the build finished"))).
		add(111, systemRec("turn_duration")).
		add(200, promptRec(relayedTeammateMessage("worker", "The reader is done."))).
		add(210, assistantRec(textBlock("thanks"))).
		add(211, systemRec("turn_duration")).
		add(300, promptRec("carry on")).
		add(310, assistantRec(textBlock("carrying on"))).
		// No turn end, no input: nothing on record says what this gap was waiting for.
		add(4000, assistantRec(textBlock("back again"))).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	requireKinds(t, rows,
		KindWriting, KindWaitingForTask, KindWriting, KindWaitingForTeammate, KindWriting,
		KindWaitingForPerson, KindWriting, KindWaitingUnknown, KindWriting)
	checkTiling(t, rows, at(0), at(4000))

	totals := (&Timeline{Rows: rows}).TotalsByKind()
	for kind, want := range map[Kind]time.Duration{
		KindWaitingForTask:     95 * time.Second,
		KindWaitingForTeammate: 90 * time.Second,
		KindWaitingForPerson:   90 * time.Second,
		KindWaitingUnknown:     3690 * time.Second,
	} {
		if got := totals[kind]; got != want {
			t.Errorf("%s totals %s, want %s:\n%s", kind, FormatDuration(got), FormatDuration(want), dump(rows))
		}
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

	requireKinds(t, rows, KindWriting, KindWaitingForTask, KindThinking, KindWriting)
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

	requireKinds(t, rows, KindWriting, KindWaitingUnknown, KindThinking, KindWriting)
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
	wait := rowOfKind(t, rows, KindWaitingForTeammate)

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
	wait := rowOfKind(t, rows, KindWaitingForPerson)

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
	wait := rowOfKindIn(t, rows, KindWaitingForTeammate, "worker")

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
	return rowOfKindIn(t, rows, kind, "")
}

// rowOfKindIn returns the first row of a kind, optionally from one lane.
func rowOfKindIn(t *testing.T, rows []Row, kind Kind, agent string) Row {
	t.Helper()
	for _, r := range rows {
		if r.Kind == kind && (agent == "" || r.Agent == agent) {
			return r
		}
	}
	t.Fatalf("no %s row for %q:\n%s", kind, agent, dump(rows))
	return Row{}
}

// TestQueuedInputSplitsEvenWithoutATurnEnding covers a lane the harness never wrote a turn-end record for. Real
// example: a text block, then nothing for 7h23m, then a queued "Go on" and a thinking block three seconds later. With
// the turn ending as the only signal, those 7h23m were reported as thinking.
//
// So input arriving splits the lane whenever no tool call is open, whatever the harness recorded. The cost is that
// input arriving while the agent is genuinely composing clips a few seconds off the front of that row, which is a
// bounded error where the other one has no bound at all.
func TestQueuedInputSplitsEvenWithoutATurnEnding(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("Here's the plan."))).
		add(26600, queueRec("enqueue", "Go on")).
		add(26603, assistantRec(thinkingBlock(""))).
		add(26610, assistantRec(textBlock("Carrying on."))).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	requireKinds(t, rows, KindWriting, KindWaitingForPerson, KindThinking, KindWriting)
	checkTiling(t, rows, at(0), at(26610))
	if got := rows[1].Duration(); got != 26595*time.Second {
		t.Errorf("the wait lasted %s, want the whole stretch until the input arrived", got)
	}
	if got := rows[2].Duration(); got != 3*time.Second {
		t.Errorf("thinking lasted %s, want the 3s from the input to the block", got)
	}
}

// TestImplausiblyLongResponseIsIdleTime is the backstop for a lane with no evidence at all. Real example: a session
// left after `/exit` and resumed 25 days later, whose first record on resume is a text block. Nothing in the lane says
// when the resume happened, and 596 hours is not a model writing a paragraph.
func TestImplausiblyLongResponseIsIdleTime(t *testing.T) {
	lane := newLane("lead", true).
		add(0, promptRec("go")).
		add(5, assistantRec(textBlock("Goodbye."))).
		add(2200000, assistantRec(textBlock("Back again."))).
		add(2200010, assistantRec(textBlock("Where were we?"))).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows

	requireKinds(t, rows, KindWriting, KindWaitingUnknown, KindWriting, KindWriting)
	checkTiling(t, rows, at(0), at(2200010))
	if rows[2].Duration() != 0 {
		t.Errorf("the block that closed the gap can't claim it: %s", rowSummary(rows[2]))
	}
	if !strings.Contains(rows[1].Info, "idle") {
		t.Errorf("the wait says %q, want it to read as idle time", rows[1].Info)
	}
}
