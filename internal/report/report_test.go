package report_test

import (
	"testing"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/report"
	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

var base = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

func span(offset, length time.Duration) (time.Time, time.Time) {
	return base.Add(offset), base.Add(offset + length)
}

// ladderGroup is the tool group the fixture's three tool rows belong to, so one fixture holds both properties this file
// exists to guard.
const ladderGroup = "Bash (file write)"

// ladderLengths gives every activity kind a distinct length, so a total that picked up the wrong kind can be read
// straight off the failure rather than worked backwards from a sum.
//
// Lane time is 1,280 s, net is 980 s, and active is 170 s.
var ladderLengths = map[timeline.Kind]time.Duration{
	timeline.KindThinking:           10 * time.Second,
	timeline.KindWriting:            20 * time.Second,
	timeline.KindToolCall:           30 * time.Second,
	timeline.KindToolExecution:      40 * time.Second,
	timeline.KindWaitingForPerson:   100 * time.Second,
	timeline.KindWaitingForTeammate: 200 * time.Second,
	timeline.KindWaitingForTask:     300 * time.Second,
	timeline.KindWaitingUnknown:     400 * time.Second,
	timeline.KindAPIError:           50 * time.Second,
	timeline.KindStalled:            60 * time.Second,
	timeline.KindCompacting:         70 * time.Second,
}

// ladderTimeline is one lane holding a row of every kind, in legend order, tiling its span.
func ladderTimeline(t *testing.T) *timeline.Timeline {
	t.Helper()

	var (
		rows   []timeline.Row
		offset time.Duration
	)
	for _, kind := range timeline.Kinds {
		length, ok := ladderLengths[kind]
		if !ok {
			t.Fatalf(`The %q kind has no length in ladderLengths, so the ladder isn't being checked against it.

Give it one, then decide which rung it belongs to: does net keep it (this lane's own clock) or drop it (a person's or a
teammate's), and does active keep it (the lane was producing something) or drop it?`, kind)
		}
		from, until := span(offset, length)
		row := timeline.Row{From: from, Until: until, LaneID: "lead", Agent: "lead", Kind: kind}
		// The three kinds a tool call can leave behind all carry the group's name, which is the trap the second test
		// below pins.
		if kind == timeline.KindToolCall || kind == timeline.KindToolExecution || kind == timeline.KindStalled {
			row.Tool, row.Class, row.ToolGroup, row.ToolLeaf = "Bash", "file write", ladderGroup, "rm"
		}
		rows = append(rows, row)
		offset += length
	}
	return &timeline.Timeline{Rows: rows, First: base, Last: base.Add(offset)}
}

// TestTheLadderHoldsFromLaneTimeDownToActive is the guard on five confusable durations. Each rung is the one above minus
// something, and the arithmetic is the definition:
//
//	lane time  every lane's clock added up
//	net        minus waiting for a person, minus waiting for a teammate
//	active     minus stalls, API errors, background-task waits, and unknown waits
//
// Net exists because a wait on a teammate is already counted as that teammate's own lane time, so an "agent time" total
// holding it counts the same work twice, and a wait on a person was never agent time. Active answers a different
// question, "how much was actually producing", so the two aren't rivals and neither replaces the other.
func TestTheLadderHoldsFromLaneTimeDownToActive(t *testing.T) {
	body := report.ForTimeline(session.Summary{ID: "s"}, ladderTimeline(t), false)
	totals := body.Totals

	seconds := func(kinds ...timeline.Kind) float64 {
		var sum time.Duration
		for _, kind := range kinds {
			sum += ladderLengths[kind]
		}
		return sum.Seconds()
	}

	var laneTime float64
	for _, kind := range timeline.Kinds {
		laneTime += seconds(kind)
	}
	if got := totals.LaneTimeSeconds; got != laneTime {
		t.Fatalf("lane time = %v, want %v: lane time is every row of every lane, whatever kind it is", got, laneTime)
	}

	wantNet := laneTime - seconds(timeline.KindWaitingForPerson, timeline.KindWaitingForTeammate)
	if got := totals.NetSeconds; got != wantNet {
		t.Errorf(`net = %v, want %v.

Net is lane time minus waiting for a person and waiting for a teammate, and nothing else: it keeps stalls, API errors,
background-task waits, unknown waits, and compacting, because that clock is this lane's own. Lane time here is %v.`,
			got, wantNet, laneTime)
	}

	wantActive := wantNet - seconds(timeline.KindWaitingForTask, timeline.KindWaitingUnknown,
		timeline.KindStalled, timeline.KindAPIError)
	if got := totals.ActiveSeconds; got != wantActive {
		t.Errorf(`active = %v, want %v.

Active is net minus the gaps net keeps: background-task waits, unknown waits, stalls, and API errors. Net here is %v,
so a mismatch means either active took the wrong kinds out or net did.`, got, wantActive, totals.NetSeconds)
	}

	if !(totals.LaneTimeSeconds >= totals.NetSeconds && totals.NetSeconds >= totals.ActiveSeconds) {
		t.Errorf(`the ladder isn't ordered: lane time %v, net %v, active %v.

Every rung is the one above minus a non-negative duration, so this ordering can't break unless one of the three is
adding something the rung above it doesn't have.`, totals.LaneTimeSeconds, totals.NetSeconds, totals.ActiveSeconds)
	}
}

// TestAToolGroupsThreeClocksAccountForEveryRowCarryingItsName pins the other half of the definition. A call leaves a row
// for the agent composing it and a row for the tool, and a stalled call leaves that second row under another kind. All
// three carry the group's name, so a breakdown that adds them into one number reports the tool as costing what the agent
// and a suspension cost, and nothing in the output looks wrong.
func TestAToolGroupsThreeClocksAccountForEveryRowCarryingItsName(t *testing.T) {
	tl := ladderTimeline(t)
	body := report.ForTimeline(session.Summary{ID: "s"}, tl, false)

	if len(body.Totals.ByTool) != 1 {
		t.Fatalf("wanted one group, got %+v", body.Totals.ByTool)
	}
	group := body.Totals.ByTool[0]
	if group.Group != ladderGroup {
		t.Fatalf("group = %q, want %q", group.Group, ladderGroup)
	}

	for _, c := range []struct {
		what string
		got  float64
		want time.Duration
		why  string
	}{
		{"seconds", group.Seconds, ladderLengths[timeline.KindToolExecution],
			"`seconds` is the tool running, so the stalled call and the agent composing the calls are both out of it"},
		{"composingSeconds", group.ComposingSeconds, ladderLengths[timeline.KindToolCall],
			"`composingSeconds` is the agent's clock writing the call, which for an `Edit` is most of what the call cost"},
		{"stalledSeconds", group.StalledSeconds, ladderLengths[timeline.KindStalled],
			"`stalledSeconds` is a call that came back far too late to have been running, kept apart so one suspension doesn't read as the tool being slow"},
	} {
		if c.got != c.want.Seconds() {
			t.Errorf("%s = %v, want %v: %s", c.what, c.got, c.want.Seconds(), c.why)
		}
	}

	// Nothing may fall between the three clocks: every row carrying the group's name lands in exactly one of them.
	var carrying float64
	for _, r := range tl.Rows {
		if r.ToolGroup == ladderGroup {
			carrying += r.Duration().Seconds()
		}
	}
	if summed := group.Seconds + group.ComposingSeconds + group.StalledSeconds; summed != carrying {
		t.Errorf(`the group's three clocks add to %v, but %v of rows carry its name.

`+"`seconds`"+` plus `+"`composingSeconds`"+` plus `+"`stalledSeconds`"+` has to account for every row the grouping rule
put in the group, or a kind the derivation added is being counted nowhere.`, summed, carrying)
	}
	if got, want := group.Calls, 2; got != want {
		t.Errorf("calls = %d, want %d: a stalled call was still a call, and the composing row still isn't one", got, want)
	}
}

func TestActiveSecondsIsLaneTimeWithTheWaitingTakenOut(t *testing.T) {
	// A minute thinking, a minute of tool, and an hour waiting on a person. "Net time building this" is the two
	// minutes, and it's a named number so nobody has to work it out from the pie.
	rows := []timeline.Row{}
	for _, c := range []struct {
		kind   timeline.Kind
		offset time.Duration
		length time.Duration
	}{
		{timeline.KindThinking, 0, time.Minute},
		{timeline.KindToolExecution, time.Minute, time.Minute},
		{timeline.KindWaitingForPerson, 2 * time.Minute, time.Hour},
		{timeline.KindStalled, 62 * time.Minute, 10 * time.Minute},
	} {
		from, until := span(c.offset, c.length)
		rows = append(rows, timeline.Row{From: from, Until: until, LaneID: "lead", Agent: "lead", Kind: c.kind})
	}

	tl := &timeline.Timeline{Rows: rows, First: base, Last: base.Add(72 * time.Minute)}
	body := report.ForTimeline(session.Summary{ID: "s"}, tl, false)

	if got, want := body.Totals.LaneTimeSeconds, float64(72*60); got != want {
		t.Errorf("lane time = %v, want %v", got, want)
	}
	if got, want := body.Totals.ActiveSeconds, float64(2*60); got != want {
		t.Errorf("active = %v, want %v: waiting and stalls aren't work", got, want)
	}
}

func TestAToolsSecondsLeaveOutTheAgentComposingTheCall(t *testing.T) {
	compose := timeline.Row{LaneID: "lead", Agent: "lead", Kind: timeline.KindToolCall,
		Tool: "Bash", Class: "checker", ToolGroup: "Bash (checker)", ToolLeaf: "pnpm check"}
	compose.From, compose.Until = span(0, 30*time.Second)

	run := timeline.Row{LaneID: "lead", Agent: "lead", Kind: timeline.KindToolExecution,
		Tool: "Bash", Class: "checker", ToolGroup: "Bash (checker)", ToolLeaf: "pnpm check"}
	run.From, run.Until = span(30*time.Second, 2*time.Minute)

	tl := &timeline.Timeline{Rows: []timeline.Row{compose, run}, First: base, Last: base.Add(150 * time.Second)}
	body := report.ForTimeline(session.Summary{ID: "s"}, tl, false)

	if len(body.Totals.ByTool) != 1 {
		t.Fatalf("wanted one group, got %d", len(body.Totals.ByTool))
	}
	group := body.Totals.ByTool[0]
	if got, want := group.Calls, 1; got != want {
		t.Errorf("calls = %d, want %d", got, want)
	}
	if got, want := group.Seconds, float64(120); got != want {
		t.Errorf("seconds = %v, want %v: the 30s composing the call is the agent's, not the tool's", got, want)
	}
	if got, want := body.Totals.LaneTimeSeconds, float64(150); got != want {
		t.Errorf("lane time = %v, want %v: lane time still holds both rows", got, want)
	}
}

func TestASessionWithNoTimestampsReportsNullRatherThanYearOne(t *testing.T) {
	body := report.ForTimeline(session.Summary{ID: "s"}, &timeline.Timeline{}, true)
	if body.Totals.From != nil || body.Totals.Until != nil {
		t.Errorf("from/until = %v/%v, want null", body.Totals.From, body.Totals.Until)
	}
	if body.Totals.Rows != 0 || len(body.Rows) != 0 {
		t.Errorf("wanted no rows, got %d", body.Totals.Rows)
	}
}
