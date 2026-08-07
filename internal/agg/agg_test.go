package agg_test

import (
	"testing"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/agg"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

// stockholm is the zone the day buckets are cut in, so a test can say "this row crossed midnight" and mean it.
var stockholm = time.FixedZone("CEST", 2*60*60)

func at(day, clock string) time.Time {
	t, err := time.Parse(time.RFC3339, day+"T"+clock+"Z")
	if err != nil {
		panic(err)
	}
	return t
}

// row is the shorthand the cases are written in: a lane, a kind, and a span.
func row(lane, kind string, from, until time.Time) timeline.Row {
	return timeline.Row{From: from, Until: until, LaneID: lane, Agent: lane, Kind: timeline.Kind(kind)}
}

func toolRow(lane, group, leaf, class string, from, until time.Time) timeline.Row {
	r := row(lane, string(timeline.KindToolExecution), from, until)
	r.Tool = "Bash"
	r.Class = timeline.ToolClass(class)
	r.ToolGroup = group
	r.ToolLeaf = leaf
	return r
}

func build(rows []timeline.Row) *agg.Cube {
	return agg.Build(&timeline.Timeline{Rows: rows}, agg.Options{Zone: stockholm})
}

// find returns the one cell matching want, and fails when the roll-up produced none or several.
func find(t *testing.T, cells []agg.Cell, match func(agg.Cell) bool) agg.Cell {
	t.Helper()
	var found []agg.Cell
	for _, c := range cells {
		if match(c) {
			found = append(found, c)
		}
	}
	if len(found) != 1 {
		t.Fatalf("wanted exactly one matching cell, got %d of %d: %+v", len(found), len(cells), cells)
	}
	return found[0]
}

func TestRollingUpOverEveryDimensionGivesOneCellHoldingTheWholeSession(t *testing.T) {
	cube := build([]timeline.Row{
		row("lead", "thinking", at("2026-08-03", "10:00:00"), at("2026-08-03", "10:00:30")),
		row("lead", "writing", at("2026-08-03", "10:00:30"), at("2026-08-03", "10:00:40")),
		row("a1", "thinking", at("2026-08-03", "10:00:00"), at("2026-08-03", "10:01:00")),
	})

	cells := cube.RollUp(0)
	if len(cells) != 1 {
		t.Fatalf("rolling up over everything should give one cell, got %d", len(cells))
	}
	if got, want := cells[0].Duration, 100*time.Second; got != want {
		t.Errorf("total duration = %v, want %v", got, want)
	}
	if got, want := cells[0].Rows, 3; got != want {
		t.Errorf("total rows = %d, want %d", got, want)
	}
	if got, want := cells[0].Lanes, 2; got != want {
		t.Errorf("lanes = %d, want %d", got, want)
	}
}

func TestOnlyTheRowAToolRanInCountsAsACall(t *testing.T) {
	// Every call leaves two rows: the agent composing it, and the tool running. Counting both reports every call twice.
	compose := row("lead", string(timeline.KindToolCall), at("2026-08-03", "10:00:00"), at("2026-08-03", "10:00:01"))
	compose.Tool = "Bash"
	compose.Class = timeline.ToolClass("checker")
	compose.ToolGroup = "Bash (checker)"
	compose.ToolLeaf = "pnpm check"

	cube := build([]timeline.Row{
		compose,
		toolRow("lead", "Bash (checker)", "pnpm check", "checker",
			at("2026-08-03", "10:00:01"), at("2026-08-03", "10:02:01")),
	})

	cell := find(t, cube.RollUp(agg.ByGroup), func(c agg.Cell) bool { return c.Group == "Bash (checker)" })
	if got, want := cell.Calls, 1; got != want {
		t.Errorf("calls = %d, want %d: the composing row isn't a call", got, want)
	}
	if got, want := cell.Rows, 2; got != want {
		t.Errorf("rows = %d, want %d: both rows are still rows", got, want)
	}
	if got, want := cell.Duration, 121*time.Second; got != want {
		t.Errorf("duration = %v, want %v", got, want)
	}
}

func TestALaneCountIsDistinctRatherThanSummed(t *testing.T) {
	// One lane calling two of a server's methods is one lane for the server, and one for each method.
	cube := build([]timeline.Row{
		toolRow("lead", "codegraph (MCP)", "codegraph_search", "mcp",
			at("2026-08-03", "10:00:00"), at("2026-08-03", "10:00:01")),
		toolRow("lead", "codegraph (MCP)", "codegraph_explore", "mcp",
			at("2026-08-03", "10:00:01"), at("2026-08-03", "10:00:02")),
		toolRow("a1", "codegraph (MCP)", "codegraph_search", "mcp",
			at("2026-08-03", "10:00:02"), at("2026-08-03", "10:00:03")),
	})

	group := find(t, cube.RollUp(agg.ByGroup), func(c agg.Cell) bool { return c.Group == "codegraph (MCP)" })
	if got, want := group.Lanes, 2; got != want {
		t.Errorf("group lanes = %d, want %d", got, want)
	}
	explore := find(t, cube.RollUp(agg.ByGroup|agg.ByLeaf), func(c agg.Cell) bool { return c.Leaf == "codegraph_explore" })
	if got, want := explore.Lanes, 1; got != want {
		t.Errorf("leaf lanes = %d, want %d", got, want)
	}
}

func TestARowCrossingMidnightSplitsItsSecondsButNotItsCount(t *testing.T) {
	// 23:30 to 00:30 local, which is 21:30 to 22:30 UTC in a +02:00 zone.
	cube := build([]timeline.Row{
		row("lead", "tool execution", at("2026-08-03", "21:30:00"), at("2026-08-03", "22:30:00")),
	})

	days := cube.RollUp(agg.ByDay)
	if len(days) != 2 {
		t.Fatalf("wanted the row split across two days, got %d: %+v", len(days), days)
	}

	first := find(t, days, func(c agg.Cell) bool { return c.Day == "2026-08-03" })
	second := find(t, days, func(c agg.Cell) bool { return c.Day == "2026-08-04" })
	if got, want := first.Duration, 30*time.Minute; got != want {
		t.Errorf("first day duration = %v, want %v", got, want)
	}
	if got, want := second.Duration, 30*time.Minute; got != want {
		t.Errorf("second day duration = %v, want %v", got, want)
	}
	if got, want := first.Rows, 1; got != want {
		t.Errorf("first day rows = %d, want %d", got, want)
	}
	if got, want := second.Rows, 0; got != want {
		t.Errorf("second day rows = %d, want %d: a split row is still one row, counted where it started", got, want)
	}

	// Rolling the day dimension away has to give the row back whole, or every total downstream stops adding up.
	whole := cube.RollUp(0)
	if got, want := whole[0].Duration, time.Hour; got != want {
		t.Errorf("rolled-up duration = %v, want %v", got, want)
	}
	if got, want := whole[0].Rows, 1; got != want {
		t.Errorf("rolled-up rows = %d, want %d", got, want)
	}
}

func TestAZeroLengthRowStillLandsOnItsDay(t *testing.T) {
	instant := at("2026-08-03", "10:00:00")
	cube := build([]timeline.Row{row("lead", "tool call", instant, instant)})

	cells := cube.RollUp(agg.ByDay)
	if len(cells) != 1 {
		t.Fatalf("wanted one day, got %d", len(cells))
	}
	if got, want := cells[0].Day, "2026-08-03"; got != want {
		t.Errorf("day = %q, want %q", got, want)
	}
	if got, want := cells[0].Rows, 1; got != want {
		t.Errorf("rows = %d, want %d", got, want)
	}
}

func TestErrorsAndTimeoutsCountOnTheCallRatherThanTheRow(t *testing.T) {
	failed := toolRow("lead", "Bash (test)", "go test", "test",
		at("2026-08-03", "10:00:00"), at("2026-08-03", "10:00:10"))
	failed.IsError = true
	timedOut := toolRow("lead", "Bash (test)", "go test", "test",
		at("2026-08-03", "10:00:10"), at("2026-08-03", "10:00:20"))
	timedOut.TimedOut = true

	cube := build([]timeline.Row{failed, timedOut})
	cell := find(t, cube.RollUp(agg.ByGroup), func(c agg.Cell) bool { return c.Group == "Bash (test)" })
	if got, want := cell.Errors, 1; got != want {
		t.Errorf("errors = %d, want %d", got, want)
	}
	if got, want := cell.TimedOut, 1; got != want {
		t.Errorf("timed out = %d, want %d", got, want)
	}
}

func TestRollUpIsSortedSoTwoRunsAgree(t *testing.T) {
	cube := build([]timeline.Row{
		row("b", "writing", at("2026-08-03", "10:00:00"), at("2026-08-03", "10:00:01")),
		row("a", "thinking", at("2026-08-03", "10:00:01"), at("2026-08-03", "10:00:02")),
	})

	first := cube.RollUp(agg.ByLane | agg.ByKind)
	second := cube.RollUp(agg.ByLane | agg.ByKind)
	if len(first) != 2 {
		t.Fatalf("wanted two cells, got %d", len(first))
	}
	if first[0].Lane != "a" || second[0].Lane != "a" {
		t.Errorf("roll-up isn't sorted: %+v then %+v", first, second)
	}
}
