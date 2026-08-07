package stats_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/agg"
	"github.com/vdavid/claude-session-analyzer/internal/stats"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

func at(day, clock string) time.Time {
	t, err := time.Parse(time.RFC3339, day+"T"+clock+"Z")
	if err != nil {
		panic(err)
	}
	return t
}

func row(lane, kind string, from, until time.Time) timeline.Row {
	return timeline.Row{From: from, Until: until, LaneID: lane, Agent: lane, Kind: timeline.Kind(kind)}
}

// call is what one tool call actually leaves behind: the row where the agent composed it, and the row where the tool
// ran. Both carry the tool's name, which is the trap every tool question has to step over.
func call(lane, tool, group, leaf, class string, from time.Time, compose, run time.Duration) []timeline.Row {
	composing := timeline.Row{
		From: from, Until: from.Add(compose),
		LaneID: lane, Agent: lane, Kind: timeline.KindToolCall,
		Tool: tool, Class: timeline.ToolClass(class), ToolGroup: group, ToolLeaf: leaf,
	}
	running := composing
	running.Kind = timeline.KindToolExecution
	running.From = composing.Until
	running.Until = composing.Until.Add(run)
	return []timeline.Row{composing, running}
}

func cells(rows []timeline.Row) []agg.Cell {
	return agg.Build(&timeline.Timeline{Rows: rows}, agg.Options{}).Cells()
}

// withoutLanes is the grain a session digest is stored at: every dimension except the lane, whose distinct count comes
// along on the cell. A query that names a lane against these can't answer, and has to say so.
func withoutLanes(source stats.Source) stats.Source {
	dims := agg.ByKind | agg.ByClass | agg.ByGroup | agg.ByLeaf | agg.ByTool | agg.ByDay
	source.Cells = agg.RollUp(source.Cells, dims)
	return source
}

// sessionOne is a two-lane session: a lead that ran the checker and waited on a person, and a subagent that reached for
// codegraph and grep and then waited on the lead.
//
// Lane time is 343 s (lead 273, subagent 70), of which 180 s is waiting, so 163 s is active.
func sessionOne() stats.Source {
	const day = "2026-08-03"
	var rows []timeline.Row
	add := func(rs ...timeline.Row) { rows = append(rows, rs...) }

	add(row("lead", "thinking", at(day, "10:00:00"), at(day, "10:00:30")))
	add(call("lead", "Bash", "Bash (checker)", "pnpm check", "checker",
		at(day, "10:00:30"), time.Second, 120*time.Second)...)
	add(row("lead", "waiting for a person", at(day, "10:02:31"), at(day, "10:04:31")))
	add(call("lead", "Grep", "Grep", "Grep", "search", at(day, "10:04:31"), 0, 2*time.Second)...)

	add(call("a1", "mcp__codegraph__codegraph_search", "codegraph (MCP)", "codegraph_search", "mcp",
		at(day, "10:00:40"), time.Second, 5*time.Second)...)
	add(call("a1", "Grep", "Grep", "Grep", "search", at(day, "10:00:46"), time.Second, 3*time.Second)...)
	add(row("a1", "waiting for a teammate", at(day, "10:00:50"), at(day, "10:01:50")))

	return stats.Source{
		SessionID: "s1", ProjectName: "cmdr", ProjectSlug: "-Users-me-projects-cmdr", Title: "Ship the file pane",
		Cells: cells(rows), WallClockSeconds: 300, Lanes: 2,
	}
}

// sessionTwo is a one-lane session on the day before, in another project: a checker run, some thinking, and a codegraph
// call that came back an error. Lane time is 74 s, all of it active.
func sessionTwo() stats.Source {
	const day = "2026-08-02"
	var rows []timeline.Row
	add := func(rs ...timeline.Row) { rows = append(rows, rs...) }

	add(call("lead", "Bash", "Bash (checker)", "pnpm check", "checker",
		at(day, "09:00:00"), time.Second, 60*time.Second)...)
	add(row("lead", "thinking", at(day, "09:01:01"), at(day, "09:01:11")))
	failed := call("lead", "mcp__codegraph__codegraph_search", "codegraph (MCP)", "codegraph_search", "mcp",
		at(day, "09:01:11"), time.Second, 2*time.Second)
	failed[1].IsError = true
	add(failed...)

	return stats.Source{
		SessionID: "s2", ProjectName: "smb2", ProjectSlug: "-Users-me-projects-smb2", Title: "Fix the share listing",
		Cells: cells(rows), WallClockSeconds: 80, Lanes: 1,
	}
}

func corpus() []stats.Source { return []stats.Source{sessionOne(), sessionTwo()} }

func run(t *testing.T, spec stats.Spec, sessions []stats.Source) stats.Result {
	t.Helper()
	result, err := stats.Run(spec, sessions)
	if err != nil {
		t.Fatalf("running the query: %v", err)
	}
	return result
}

func where(field stats.Dim, values ...string) stats.Clause {
	return stats.Clause{Field: field, Values: values}
}

func closeTo(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

func group(t *testing.T, result stats.Result, dim stats.Dim, value string) stats.Group {
	t.Helper()
	for _, g := range result.Groups {
		if g.Value(dim) == value {
			return g
		}
	}
	t.Fatalf("no group with %s = %q, got %+v", dim, value, result.Groups)
	return stats.Group{}
}

// "How much time, and what percentage of my agents' time, went into waiting for the checker script, and in how many
// runs?"
func TestCheckerTimeItsShareOfEachDenominatorAndHowManyRuns(t *testing.T) {
	result := run(t, stats.Spec{Where: []stats.Clause{where(stats.DimClass, "checker")}}, corpus())

	if got, want := result.Matched.Seconds, 180.0; got != want {
		t.Errorf("checker seconds = %v, want %v", got, want)
	}
	if got, want := result.Matched.Calls, 2; got != want {
		t.Errorf("checker runs = %d, want %d", got, want)
	}
	if got, want := result.Totals.LaneTimeSeconds, 417.0; got != want {
		t.Errorf("lane time = %v, want %v", got, want)
	}
	if got, want := result.Totals.ActiveSeconds, 237.0; got != want {
		t.Errorf("active = %v, want %v", got, want)
	}
	if got, want := result.Totals.WallClockSeconds, 380.0; got != want {
		t.Errorf("wall clock = %v, want %v", got, want)
	}
	closeTo(t, "share of lane time", result.Matched.ShareOfLaneTime, 180.0/417.0)
	closeTo(t, "share of active", result.Matched.ShareOfActive, 180.0/237.0)
	closeTo(t, "share of wall clock", result.Matched.ShareOfWallClock, 180.0/380.0)
	if got, want := result.Totals.Sessions, 2; got != want {
		t.Errorf("sessions = %d, want %d", got, want)
	}
	if got, want := result.Totals.Lanes, 3; got != want {
		t.Errorf("lanes = %d, want %d", got, want)
	}
}

// "Did my agents use codegraph in this session? Do they prefer codegraph or grep overall?"
func TestWhetherASessionReachedForCodegraphAndWhetherItBeatsGrepOverall(t *testing.T) {
	inOneSession := run(t, stats.Spec{
		Where:   []stats.Clause{where(stats.DimSession, "s1"), where(stats.DimGroup, "codegraph*")},
		GroupBy: []stats.Dim{stats.DimGroup},
	}, corpus())

	if len(inOneSession.Groups) != 1 {
		t.Fatalf("wanted one group, got %+v", inOneSession.Groups)
	}
	if got, want := inOneSession.Groups[0].Group, "codegraph (MCP)"; got != want {
		t.Errorf("group = %q, want %q", got, want)
	}
	if got, want := inOneSession.Matched.Calls, 1; got != want {
		t.Errorf("calls in s1 = %d, want %d", got, want)
	}
	if got, want := inOneSession.Matched.Seconds, 5.0; got != want {
		t.Errorf("seconds in s1 = %v, want %v", got, want)
	}

	overall := run(t, stats.Spec{
		Where:   []stats.Clause{where(stats.DimGroup, "codegraph*", "Grep")},
		GroupBy: []stats.Dim{stats.DimGroup},
	}, corpus())

	if len(overall.Groups) != 2 {
		t.Fatalf("wanted two groups, got %+v", overall.Groups)
	}
	// Biggest first: codegraph cost 7 s over two sessions, grep 5 s over two lanes of one.
	if got, want := overall.Groups[0].Group, "codegraph (MCP)"; got != want {
		t.Errorf("first group = %q, want %q: groups are sorted by seconds descending", got, want)
	}
	codegraph := group(t, overall, stats.DimGroup, "codegraph (MCP)")
	if got, want := codegraph.Seconds, 7.0; got != want {
		t.Errorf("codegraph seconds = %v, want %v", got, want)
	}
	if got, want := codegraph.Calls, 2; got != want {
		t.Errorf("codegraph calls = %d, want %d", got, want)
	}
	if got, want := codegraph.Errors, 1; got != want {
		t.Errorf("codegraph errors = %d, want %d", got, want)
	}
	if got, want := codegraph.Lanes, 2; got != want {
		t.Errorf("codegraph lanes = %d, want %d: one lane in each session, added up", got, want)
	}
	grep := group(t, overall, stats.DimGroup, "Grep")
	if got, want := grep.Seconds, 5.0; got != want {
		t.Errorf("grep seconds = %v, want %v", got, want)
	}
	if got, want := grep.Lanes, 2; got != want {
		t.Errorf("grep lanes = %d, want %d: both lanes of one session", got, want)
	}
	closeTo(t, "codegraph share of lane time", codegraph.ShareOfLaneTime, 7.0/417.0)

	// The groups have to add back up to what matched, or one of the two numbers is lying.
	var summed float64
	for _, g := range overall.Groups {
		summed += g.Seconds
	}
	closeTo(t, "groups summed", summed, overall.Matched.Seconds)
}

// "What was the net time agents spent building this, excluding waiting for me and for each other?"
func TestNetTimeIsLaneTimeWithEveryGapTakenOut(t *testing.T) {
	result := run(t, stats.Spec{GroupBy: []stats.Dim{stats.DimKind}}, corpus())

	if got, want := result.Totals.ActiveSeconds, 237.0; got != want {
		t.Errorf("active seconds = %v, want %v", got, want)
	}

	// Asking for the working kinds by hand has to land on the same number, which is what makes ActiveSeconds a
	// shorthand rather than a second opinion. The composing rows count here: writing a call is work.
	byHand := run(t, stats.Spec{
		Where: []stats.Clause{where(stats.DimKind, "thinking", "writing", "tool call", "tool execution", "compacting")},
	}, corpus())
	if got, want := byHand.Matched.Seconds, result.Totals.ActiveSeconds; got != want {
		t.Errorf("working kinds = %v, want %v", got, want)
	}
	if got, want := group(t, result, stats.DimKind, "tool call").Seconds, 5.0; got != want {
		t.Errorf("tool call seconds = %v, want %v: a kind question keeps the composing rows", got, want)
	}
	waiting := group(t, result, stats.DimKind, "waiting for a person")
	if got, want := waiting.Seconds, 120.0; got != want {
		t.Errorf("waiting for a person = %v, want %v", got, want)
	}
}

// A tool question counts the row the tool ran in and not the row the agent composed the call in. Both carry the tool's
// name, so taking them all reports the checker as costing more than it did, and the output looks perfectly reasonable.
func TestAToolFilterCountsOnlyTheRowsTheToolRanIn(t *testing.T) {
	result := run(t, stats.Spec{Where: []stats.Clause{where(stats.DimClass, "checker")}}, corpus())
	if got, want := result.Matched.Seconds, 180.0; got != want {
		t.Errorf("checker seconds = %v, want %v: the two composing seconds aren't the checker's", got, want)
	}
	if got, want := result.Matched.Rows, 2; got != want {
		t.Errorf("checker rows = %d, want %d: one row per run, not two", got, want)
	}
	if len(result.Notes) == 0 {
		t.Errorf("a tool question should say it left the composing rows out, got no notes")
	}

	// The opt-out exists for the question about the agent rather than the tool, and it's visibly a different number.
	including := run(t, stats.Spec{
		Where:                []stats.Clause{where(stats.DimClass, "checker")},
		IncludeComposingRows: true,
	}, corpus())
	if got, want := including.Matched.Seconds, 182.0; got != want {
		t.Errorf("checker seconds with the composing rows = %v, want %v", got, want)
	}
	if got, want := including.Matched.Rows, 4; got != want {
		t.Errorf("rows with the composing rows = %d, want %d", got, want)
	}
}

// Dropping the composing rows drops the `tool call` kind with them, which is correct and worth pinning: a reader who
// expected it and can't find it should find a note instead.
func TestAToolFilterGroupedByKindHasNoToolCallKindLeft(t *testing.T) {
	result := run(t, stats.Spec{
		Where:   []stats.Clause{where(stats.DimClass, "checker")},
		GroupBy: []stats.Dim{stats.DimKind},
	}, corpus())

	if len(result.Groups) != 1 {
		t.Fatalf("wanted one kind, got %+v", result.Groups)
	}
	if got, want := result.Groups[0].Kind, string(timeline.KindToolExecution); got != want {
		t.Errorf("kind = %q, want %q", got, want)
	}
	for _, g := range result.Groups {
		if g.Kind == string(timeline.KindToolCall) {
			t.Errorf("a tool question shouldn't report the composing kind, got %+v", g)
		}
	}
}

func TestAnEmptyScopeAnswersZeroRatherThanDividingByZero(t *testing.T) {
	result := run(t, stats.Spec{Where: []stats.Clause{where(stats.DimClass, "checker")}}, nil)

	if result.Matched.Seconds != 0 || result.Totals.LaneTimeSeconds != 0 {
		t.Errorf("wanted zeroes over an empty scope, got %+v", result)
	}
	for label, share := range map[string]float64{
		"lane time":  result.Matched.ShareOfLaneTime,
		"active":     result.Matched.ShareOfActive,
		"wall clock": result.Matched.ShareOfWallClock,
	} {
		if share != 0 || math.IsNaN(share) {
			t.Errorf("share of %s = %v, want 0", label, share)
		}
	}
	if len(result.Notes) == 0 {
		t.Errorf("an empty scope should say so, got no notes")
	}
}

func TestMatchingIgnoresCaseGlobsAtEitherEndAndTakesSeveralValues(t *testing.T) {
	cases := []struct {
		name    string
		clause  stats.Clause
		seconds float64
	}{
		{"exact, wrong case", where(stats.DimClass, "CHECKER"), 180},
		{"prefix glob", where(stats.DimLeaf, "pnpm*"), 180},
		{"suffix glob", where(stats.DimGroup, "*(mcp)"), 7},
		{"glob at both ends", where(stats.DimTool, "*codegraph*"), 7},
		{"several values", where(stats.DimClass, "checker", "mcp"), 187},
		{"nothing matches", where(stats.DimClass, "dev server"), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := run(t, stats.Spec{Where: []stats.Clause{c.clause}}, corpus())
			if got := result.Matched.Seconds; got != c.seconds {
				t.Errorf("seconds = %v, want %v", got, c.seconds)
			}
		})
	}
}

func TestClausesAreAndedTogether(t *testing.T) {
	result := run(t, stats.Spec{Where: []stats.Clause{
		where(stats.DimClass, "checker", "mcp"),
		where(stats.DimDay, "2026-08-02"),
	}}, corpus())

	if got, want := result.Matched.Seconds, 62.0; got != want {
		t.Errorf("seconds = %v, want %v: only the second day's checker and codegraph runs", got, want)
	}
}

func TestTopKeepsTheBiggestGroupsAndSaysHowManyItCut(t *testing.T) {
	all := run(t, stats.Spec{GroupBy: []stats.Dim{stats.DimKind}}, corpus())
	if len(all.Groups) < 3 {
		t.Fatalf("wanted at least three kinds to cut from, got %+v", all.Groups)
	}
	if all.Truncated != 0 {
		t.Errorf("truncated = %d, want 0 when nothing was cut", all.Truncated)
	}

	topped := run(t, stats.Spec{GroupBy: []stats.Dim{stats.DimKind}, Top: 2}, corpus())
	if got, want := len(topped.Groups), 2; got != want {
		t.Errorf("groups = %d, want %d", got, want)
	}
	if got, want := topped.Truncated, len(all.Groups)-2; got != want {
		t.Errorf("truncated = %d, want %d", got, want)
	}
	if got, want := topped.Groups[0].Kind, all.Groups[0].Kind; got != want {
		t.Errorf("first group = %q, want %q: the biggest survives", got, want)
	}
	// Matched still covers everything, so a share can be read against the whole rather than the visible part.
	if got, want := topped.Matched.Seconds, all.Matched.Seconds; got != want {
		t.Errorf("matched seconds = %v, want %v: Top cuts the groups, not the match", got, want)
	}
}

func TestGroupingByLaneWithoutALaneDimensionLeavesANote(t *testing.T) {
	digests := []stats.Source{withoutLanes(sessionOne()), withoutLanes(sessionTwo())}
	result := run(t, stats.Spec{GroupBy: []stats.Dim{stats.DimLane}}, digests)

	if len(result.Notes) == 0 {
		t.Fatalf("wanted a note saying the lanes aren't there, got none")
	}
	if !strings.Contains(strings.ToLower(strings.Join(result.Notes, " ")), "lane") {
		t.Errorf("the note should name the lane dimension, got %q", result.Notes)
	}

	// The same query over cells that do carry lanes answers, and says nothing.
	withLanes := run(t, stats.Spec{GroupBy: []stats.Dim{stats.DimLane}}, corpus())
	if len(withLanes.Groups) != 2 {
		t.Fatalf("wanted the lead and the subagent, got %+v", withLanes.Groups)
	}
	for _, note := range withLanes.Notes {
		if strings.Contains(note, "lane") {
			t.Errorf("nothing to warn about here, got %q", note)
		}
	}
}

func TestLanesAreCountedPerSessionSoTwoSessionsAddUp(t *testing.T) {
	spec := stats.Spec{Where: []stats.Clause{where(stats.DimClass, "mcp")}}

	withLanes := run(t, spec, corpus())
	if got, want := withLanes.Matched.Lanes, 2; got != want {
		t.Errorf("lanes = %d, want %d: one lane in each of two sessions", got, want)
	}

	// A digest has already dropped the lane dimension and carries a count instead. The answer has to match.
	digests := []stats.Source{withoutLanes(sessionOne()), withoutLanes(sessionTwo())}
	fromDigests := run(t, spec, digests)
	if got, want := fromDigests.Matched.Lanes, 2; got != want {
		t.Errorf("lanes from digests = %d, want %d", got, want)
	}
	if got, want := fromDigests.Matched.Seconds, withLanes.Matched.Seconds; got != want {
		t.Errorf("seconds from digests = %v, want %v: the grain shouldn't change the answer", got, want)
	}
}

func TestGroupingBySeveralDimensionsKeepsOnlyThoseKeys(t *testing.T) {
	result := run(t, stats.Spec{
		Where:   []stats.Clause{where(stats.DimClass, "mcp")},
		GroupBy: []stats.Dim{stats.DimProject, stats.DimDay},
	}, corpus())

	if len(result.Groups) != 2 {
		t.Fatalf("wanted one group per project, got %+v", result.Groups)
	}
	first := group(t, result, stats.DimProject, "cmdr")
	if got, want := first.Day, "2026-08-03"; got != want {
		t.Errorf("day = %q, want %q", got, want)
	}
	if got, want := first.Seconds, 5.0; got != want {
		t.Errorf("seconds = %v, want %v", got, want)
	}
	if first.Kind != "" || first.Group != "" {
		t.Errorf("an ungrouped dimension should be empty, got %+v", first.Key)
	}
	if got, want := result.Scope.Projects, 2; got != want {
		t.Errorf("projects in scope = %d, want %d", got, want)
	}
	if got, want := result.Scope.FirstDay, "2026-08-02"; got != want {
		t.Errorf("first day = %q, want %q", got, want)
	}
	if got, want := result.Scope.LastDay, "2026-08-03"; got != want {
		t.Errorf("last day = %q, want %q", got, want)
	}
}

func TestAHandBuiltSpecNamingSomethingUnknownIsRefused(t *testing.T) {
	_, err := stats.Run(stats.Spec{GroupBy: []stats.Dim{"banana"}}, corpus())
	if err == nil {
		t.Fatalf("wanted a refusal naming the dimensions")
	}
	if !strings.Contains(err.Error(), string(stats.DimSession)) {
		t.Errorf("the message should list the dimensions, got %q", err)
	}
}
