package timeline

import (
	"strings"
	"testing"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// oneCall runs a lane that does nothing but issue one tool call and get its result back, and hands back the execution
// row. It's the shape every stall case wants to talk about.
func oneCall(t *testing.T, took time.Duration, block blockSpec, payload string) Row {
	t.Helper()
	call := block.build()
	result := toolResultRec("t1", "output")
	if payload != "" {
		result = withResultPayload(result, payload)
	}

	lane := newLane("agent", false).
		add(0, promptRec("go")).
		add(1, assistantRec(call)).
		add(1+took.Seconds(), result).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows
	for _, r := range rows {
		if r.Tool != "" && r.Kind != KindToolCall {
			return r
		}
	}
	t.Fatalf("no execution row:\n%s", dump(rows))
	return Row{}
}

// blockSpec describes the call under test.
type blockSpec struct {
	tool string
	key  string
	arg  string
	// timeout is the `timeout` input in milliseconds, zero when the call didn't ask for one.
	timeout int
}

func (s blockSpec) build() transcript.Block {
	b := toolUseBlock("t1", s.tool, s.key, s.arg)
	if s.timeout > 0 {
		b = withNumber(b, "timeout", s.timeout)
	}
	return b
}

// TestStallOnlyForWorkThatCannotTakeThatLong is the heart of the heuristic: a suspended agent gets called out, and
// honest slow work does not.
func TestStallOnlyForWorkThatCannotTakeThatLong(t *testing.T) {
	cases := []struct {
		name  string
		took  time.Duration
		block blockSpec
		want  Kind
	}{
		{
			// The event this tool exists for: a trivial cleanup whose result came back six hours later. The agent was
			// suspended, and reporting it as a six-hour `rm` would be a lie.
			name:  "a six-hour file removal",
			took:  6*time.Hour + 15*time.Minute,
			block: blockSpec{tool: "Bash", key: "command", arg: `rm -f "$D"/*.db 2>/dev/null; ls "$D"; du -sh "$D"`},
			want:  KindStalled,
		},
		{
			name:  "a forty-minute build",
			took:  40 * time.Minute,
			block: blockSpec{tool: "Bash", key: "command", arg: "cargo build --release"},
			want:  KindToolExecution,
		},
		{
			name:  "a two-hour test suite",
			took:  2 * time.Hour,
			block: blockSpec{tool: "Bash", key: "command", arg: "cargo nextest run --workspace"},
			want:  KindToolExecution,
		},
		{
			name:  "a six-minute checker run",
			took:  6 * time.Minute,
			block: blockSpec{tool: "Bash", key: "command", arg: "git add -A && pnpm check -q clippy"},
			want:  KindToolExecution,
		},
		{
			name:  "a dev server left running for three hours",
			took:  3 * time.Hour,
			block: blockSpec{tool: "Bash", key: "command", arg: "pnpm dev"},
			want:  KindToolExecution,
		},
		{
			name:  "an hour of grep",
			took:  70 * time.Minute,
			block: blockSpec{tool: "Bash", key: "command", arg: "grep -rn TODO ."},
			want:  KindStalled,
		},
		{
			name:  "a file read that took two hours",
			took:  2 * time.Hour,
			block: blockSpec{tool: "Read", key: "file_path", arg: "/tmp/notes.md"},
			want:  KindStalled,
		},
		{
			name:  "a cheap call just under the threshold",
			took:  59 * time.Minute,
			block: blockSpec{tool: "Bash", key: "command", arg: "ls /tmp"},
			want:  KindToolExecution,
		},
		{
			// A wait loop is doing exactly what it was asked to, however long it blocks, so it takes the generous
			// threshold. Past that even a wait loop was suspended.
			name:  "a wait loop blocking for its full timeout",
			took:  10 * time.Minute,
			block: blockSpec{tool: "Bash", key: "command", arg: "until pgrep -f app; do sleep 3; done", timeout: 600000},
			want:  KindToolExecution,
		},
		{
			name:  "a wait loop that came back after thirteen hours",
			took:  13 * time.Hour,
			block: blockSpec{tool: "Bash", key: "command", arg: "until pgrep -f app; do sleep 3; done"},
			want:  KindStalled,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := oneCall(t, c.took, c.block, "")
			if row.Kind != c.want {
				t.Errorf("got %q, want %q: %s", row.Kind, c.want, row.Info)
			}
			if c.want == KindStalled && !strings.Contains(row.Info, "no result for") {
				t.Errorf("a stalled row should say how long it waited, got %q", row.Info)
			}
		})
	}
}

// TestStallKeepsTheCallItWasAbout checks a stalled row still names the tool, so a reader can judge the call for
// themselves rather than taking the heuristic's word for it.
func TestStallKeepsTheCallItWasAbout(t *testing.T) {
	row := oneCall(t, 7*time.Hour, blockSpec{tool: "Bash", key: "command", arg: "rm -rf /tmp/scratch"}, "")

	if row.Tool != "Bash" || row.Class != ClassFileWrite {
		t.Errorf("got tool %q class %q, want Bash and file write", row.Tool, row.Class)
	}
	if !strings.Contains(row.Info, "rm -rf /tmp/scratch") {
		t.Errorf("the row should carry the command, got %q", row.Info)
	}
}

// TestTimeoutFromThePayload covers the harness recording the timeout it enforced, which is the reliable signal.
func TestTimeoutFromThePayload(t *testing.T) {
	row := oneCall(t, 600*time.Second,
		blockSpec{tool: "Bash", key: "command", arg: "until pgrep -f app; do sleep 3; done", timeout: 600000},
		`{"stdout":"","stderr":"","interrupted":false,"timedOutAfterMs":600000}`)

	if !row.TimedOut {
		t.Fatalf("the row should be marked as timed out: %s", row.Info)
	}
	if !strings.Contains(row.Info, "timed out after 10m00s") {
		t.Errorf("the row should say what the timeout was, got %q", row.Info)
	}
}

// TestTimeoutFromTheRequestedLimit covers the other shape the same event takes: the payload is a bare error string, so
// the only evidence is that the call ran for exactly as long as it was allowed to.
func TestTimeoutFromTheRequestedLimit(t *testing.T) {
	row := oneCall(t, 600*time.Second+100*time.Millisecond,
		blockSpec{tool: "Bash", key: "command", arg: "until nc -z 127.0.0.1 19427; do sleep 5; done", timeout: 600000}, "")

	if !row.TimedOut {
		t.Fatalf("a call that ran for its whole timeout timed out: %s", row.Info)
	}
}

// TestTimeoutIsCappedByTheHarness covers a call asking for half an hour and being cut at ten minutes, which is the
// harness's own ceiling rather than the one the agent asked for.
func TestTimeoutIsCappedByTheHarness(t *testing.T) {
	row := oneCall(t, 600*time.Second+100*time.Millisecond,
		blockSpec{tool: "Bash", key: "command", arg: "until uptime; do sleep 30; done", timeout: 1800000}, "")

	if !row.TimedOut {
		t.Fatalf("the harness caps Bash at ten minutes whatever the call asked for: %s", row.Info)
	}
}

// TestOvershootingTheLimitIsNotATimeout covers the call that ran well past the timeout it asked for and still came
// back with output. Something other than the timeout ended it, so saying it timed out would be wrong.
func TestOvershootingTheLimitIsNotATimeout(t *testing.T) {
	row := oneCall(t, 471*time.Second,
		blockSpec{tool: "Bash", key: "command", arg: "pkill -f app; sleep 3; pgrep -f app | wc -l", timeout: 300000}, "")

	if row.TimedOut {
		t.Errorf("running 171s past a 300s limit is not a timeout: %s", row.Info)
	}
}

// TestNoTimeoutGuessWithoutALimit covers a call that never asked for a timeout. The harness default would put a
// two-minute command right on the line, and guessing there would put words in the transcript's mouth.
func TestNoTimeoutGuessWithoutALimit(t *testing.T) {
	row := oneCall(t, 120*time.Second, blockSpec{tool: "Bash", key: "command", arg: "cargo check"}, "")

	if row.TimedOut {
		t.Errorf("without a requested timeout there's nothing to compare against: %s", row.Info)
	}
}

// TestTimedOutIsNeverStalled covers the overlap: the harness ended the call on purpose, so however long it blocked,
// the agent wasn't suspended.
func TestTimedOutIsNeverStalled(t *testing.T) {
	row := oneCall(t, 20*time.Hour,
		blockSpec{tool: "Bash", key: "command", arg: "rm -rf /tmp/scratch", timeout: 72000000},
		`{"stdout":"","timedOutAfterMs":72000000}`)

	if row.Kind != KindToolExecution {
		t.Errorf("got %q, want a tool execution: %s", row.Kind, row.Info)
	}
}

// TestUnfinishedCallIsNeverStalled covers a transcript read while the tool was still running. Its end is the moment we
// stopped reading, which is not evidence of anything.
func TestUnfinishedCallIsNeverStalled(t *testing.T) {
	lane := newLane("agent", false).
		add(0, promptRec("go")).
		add(1, assistantRec(toolUseBlock("t1", "Bash", "command", "rm -rf /tmp/scratch"))).
		add(40000, systemRec("stop_hook_summary")).
		done()

	rows := Derive(sessionOf(lane), Options{}).Rows
	last := rows[len(rows)-1]
	if last.Kind != KindToolExecution {
		t.Fatalf("got %q, want a tool execution:\n%s", last.Kind, dump(rows))
	}
	if !strings.Contains(last.Info, "no result in the transcript") {
		t.Errorf("the row should say the result never arrived, got %q", last.Info)
	}
}

// TestAskingAPersonIsWaiting covers the tools that block on a human answering. Across every session on the machine
// this was built against, three of the five rows the stall rule flagged were `AskUserQuestion` calls left open for
// hours, which is not a suspended agent: it's the agent doing the one thing it's meant to do when it needs a person.
func TestAskingAPersonIsWaiting(t *testing.T) {
	for _, tool := range []string{"AskUserQuestion", "ExitPlanMode"} {
		t.Run(tool, func(t *testing.T) {
			row := oneCall(t, 26*time.Hour, blockSpec{tool: tool, key: "question", arg: "Which option?"}, "")

			if row.Kind != KindWaitingForPerson {
				t.Errorf("got %q, want a wait on a person: %s", row.Kind, row.Info)
			}
			if !strings.Contains(row.Info, "waiting for an answer to "+tool) {
				t.Errorf("the row says %q, want it to name what the answer was for", row.Info)
			}
		})
	}
}
