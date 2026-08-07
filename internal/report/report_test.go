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
