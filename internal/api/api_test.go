package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/report"
	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

// base is the instant the hand-built timelines here count from. A round UTC time, so a failure reads as an offset.
var base = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

const (
	alphaID  = "11111111-1111-1111-1111-111111111111"
	goldenID = "11111111-2222-3333-4444-555555555555"
	// goldenRows and goldenLanes are what the derivation's golden CSV holds for the fixture session.
	goldenRows  = 37
	goldenLanes = 3
)

func sessionRoot() string { return filepath.Join("..", "session", "testdata", "projects") }
func goldenRoot() string  { return filepath.Join("..", "timeline", "testdata", "projects") }

// get runs one request against a handler over the given transcript root.
func get(t *testing.T, root, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	New(Options{Root: root, FrontendOrigins: []string{"http://127.0.0.1:19428"}}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode the body: %v\n%s", err, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content type = %q, want JSON", ct)
	}
	return out
}

func TestSessionsAnswersWithTheNewestFirst(t *testing.T) {
	rec := get(t, sessionRoot(), "/api/sessions")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[report.SessionList](t, rec)

	if len(got.Sessions) != 4 {
		t.Fatalf("returned %d sessions, want the 4 in testdata", len(got.Sessions))
	}
	if !strings.HasPrefix(got.Sessions[0].ID, "33333333-bbbb") {
		t.Errorf("first session = %s, want the newest one", got.Sessions[0].ID)
	}
	if got.Totals.Sessions != 4 || got.Totals.Subagents != 4 {
		t.Errorf("totals = %+v, want 4 sessions and the 4 subagents alpha spawned", got.Totals)
	}

	var alpha report.Session
	for _, s := range got.Sessions {
		if s.ID == alphaID {
			alpha = s
		}
	}
	if alpha.Title != "Widgets, counted" {
		t.Errorf("title = %q", alpha.Title)
	}
	if alpha.ProjectPath != "/tmp/alpha" || alpha.ProjectName != "alpha" {
		t.Errorf("project = %q / %q, want the path and its last element", alpha.ProjectPath, alpha.ProjectName)
	}
	if alpha.Subagents != 4 {
		t.Errorf("subagents = %d, want 4", alpha.Subagents)
	}
	if alpha.Seconds != 5 {
		t.Errorf("seconds = %v, want 5", alpha.Seconds)
	}
	if alpha.Start == nil || alpha.End == nil {
		t.Errorf("start and end should be there: %+v", alpha)
	}
}

func TestSessionsHonoursALimitAndStillTotalsEverything(t *testing.T) {
	rec := get(t, sessionRoot(), "/api/sessions?limit=2")

	got := decode[report.SessionList](t, rec)
	if len(got.Sessions) != 2 {
		t.Errorf("returned %d sessions, want 2", len(got.Sessions))
	}
	if got.Totals.Sessions != 4 {
		t.Errorf("totals say %d sessions, want all 4: a limit caps the page, not the count", got.Totals.Sessions)
	}
}

func TestSessionsRejectsALimitThatIsntANumber(t *testing.T) {
	rec := get(t, sessionRoot(), "/api/sessions?limit=lots")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if code := decode[errorBody](t, rec).Error.Code; code != "bad_request" {
		t.Errorf("code = %q", code)
	}
}

func TestSessionsRejectsANegativeLimit(t *testing.T) {
	rec := get(t, sessionRoot(), "/api/sessions?limit=-3")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if code := decode[errorBody](t, rec).Error.Code; code != "bad_request" {
		t.Errorf("code = %q", code)
	}
}

func TestOneSessionResolvesFromAPrefix(t *testing.T) {
	rec := get(t, sessionRoot(), "/api/sessions/1111")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[struct {
		Session report.Session `json:"session"`
	}](t, rec)
	if got.Session.ID != alphaID {
		t.Errorf("id = %q, want the full id a prefix resolved to", got.Session.ID)
	}
}

func TestAnUnknownSessionIs404WithSomethingToDoNext(t *testing.T) {
	rec := get(t, sessionRoot(), "/api/sessions/no-such-session")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	got := decode[errorBody](t, rec)
	if got.Error.Code != "not_found" {
		t.Errorf("code = %q", got.Error.Code)
	}
	if !strings.Contains(got.Error.Message, "no-such-session") {
		t.Errorf("message %q should repeat the id", got.Error.Message)
	}
}

func TestAnAmbiguousIDIs400AndNamesTheCandidates(t *testing.T) {
	rec := get(t, sessionRoot(), "/api/sessions/33333333/timeline")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	got := decode[errorBody](t, rec)
	if got.Error.Code != "ambiguous_id" {
		t.Errorf("code = %q", got.Error.Code)
	}
	if len(got.Error.Matches) != 2 {
		t.Errorf("matches = %v, want both candidates so the caller can pick", got.Error.Matches)
	}
}

func TestTimelineReturnsRowsAndTheAggregatesAFrontendWouldOtherwiseSum(t *testing.T) {
	rec := get(t, goldenRoot(), "/api/sessions/"+goldenID+"/timeline")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[report.Timeline](t, rec)

	if got.Session.ID != goldenID {
		t.Errorf("session id = %q", got.Session.ID)
	}
	if len(got.Rows) != goldenRows {
		t.Errorf("rows = %d, want %d, the golden CSV's", len(got.Rows), goldenRows)
	}
	if len(got.Lanes) != goldenLanes {
		t.Errorf("lanes = %d, want %d", len(got.Lanes), goldenLanes)
	}
	if got.Totals.Rows != goldenRows || got.Totals.Lanes != goldenLanes {
		t.Errorf("totals = %+v, want them to agree with the arrays", got.Totals)
	}

	// The two numbers a reader could confuse. Lanes run in parallel, so summed lane time is the larger one, and the
	// names have to keep them apart.
	if got.Totals.WallClockSeconds <= 0 || got.Totals.LaneTimeSeconds <= 0 {
		t.Fatalf("totals = %+v, want both spans", got.Totals)
	}
	if got.Totals.LaneTimeSeconds <= got.Totals.WallClockSeconds {
		t.Errorf("lane time %v should exceed wall clock %v in a session with concurrent lanes",
			got.Totals.LaneTimeSeconds, got.Totals.WallClockSeconds)
	}

	// The pie has to add up to the rows it's made of.
	var byKind, rows float64
	for _, k := range got.Totals.ByKind {
		byKind += k.Seconds
	}
	for _, r := range got.Rows {
		rows += r.Seconds
	}
	if math.Abs(byKind-rows) > 0.01 {
		t.Errorf("per-kind totals add up to %v, the rows to %v", byKind, rows)
	}
	if math.Abs(byKind-got.Totals.LaneTimeSeconds) > 0.01 {
		t.Errorf("per-kind totals add up to %v, lane time says %v", byKind, got.Totals.LaneTimeSeconds)
	}

	for _, lane := range got.Lanes {
		var laneKinds float64
		for _, k := range lane.ByKind {
			laneKinds += k.Seconds
		}
		var laneRows float64
		var counted int
		for _, r := range got.Rows {
			if r.LaneID == lane.ID {
				laneRows += r.Seconds
				counted++
			}
		}
		if math.Abs(laneKinds-laneRows) > 0.01 {
			t.Errorf("lane %s: per-kind totals add up to %v, its rows to %v", lane.Name, laneKinds, laneRows)
		}
		if lane.Rows != counted {
			t.Errorf("lane %s: says %d rows, %d carry its id", lane.Name, lane.Rows, counted)
		}
		if lane.From.IsZero() || lane.Until.IsZero() {
			t.Errorf("lane %s: a swimlane needs both ends: %+v", lane.Name, lane)
		}
	}
}

// TestTimelineCountsToolsByGroupAndByLeaf covers the breakdown that answers "which tools did this session use, and who
// used them". It's built from a hand-made timeline rather than the fixture, because the shapes that matter are a group
// holding several leaves and a leaf reached from several lanes, and the fixture has neither.
func TestTimelineCountsToolsByGroupAndByLeaf(t *testing.T) {
	run := func(lane, tool, group, leaf string, seconds float64) timeline.Row {
		return timeline.Row{
			From: base, Until: base.Add(time.Duration(seconds * float64(time.Second))),
			LaneID: lane, Agent: lane, Kind: timeline.KindToolExecution,
			Tool: tool, Class: timeline.ClassMCP, ToolGroup: group, ToolLeaf: leaf,
		}
	}
	call := func(lane, tool, group, leaf string) timeline.Row {
		r := run(lane, tool, group, leaf, 0)
		r.Kind = timeline.KindToolCall
		return r
	}

	tl := &timeline.Timeline{
		First: base, Last: base.Add(time.Minute),
		Lanes: []timeline.LaneSpan{{ID: "lead", Name: "lead", IsLead: true, First: base, Last: base.Add(time.Minute)}},
		Rows: []timeline.Row{
			// The composing rows sit beside every run and must not be counted as calls of their own.
			call("lead", "mcp__codegraph__codegraph_search", "codegraph (MCP)", "codegraph_search"),
			run("lead", "mcp__codegraph__codegraph_search", "codegraph (MCP)", "codegraph_search", 2),
			run("worker", "mcp__codegraph__codegraph_search", "codegraph (MCP)", "codegraph_search", 3),
			run("worker", "mcp__codegraph__codegraph_node", "codegraph (MCP)", "codegraph_node", 1),
			run("lead", "Read", "Read", "Read", 4),
			// A run that failed is still a call, and the count of failures rides along with it.
			func() timeline.Row {
				r := run("lead", "Read", "Read", "Read", 10)
				r.IsError = true
				return r
			}(),
		},
	}

	got := report.ForTimeline(session.Summary{ID: "s"}, tl, false)

	if len(got.Totals.ByTool) != 2 {
		t.Fatalf("groups = %+v, want codegraph and Read", got.Totals.ByTool)
	}

	// Biggest first, so a legend reads top down.
	codegraph, read := got.Totals.ByTool[0], got.Totals.ByTool[1]
	if codegraph.Group != "codegraph (MCP)" || read.Group != "Read" {
		t.Fatalf("groups = %q then %q, want them ordered by calls", codegraph.Group, read.Group)
	}

	if codegraph.Calls != 3 || codegraph.Seconds != 6 {
		t.Errorf("codegraph = %d calls in %vs, want 3 in 6s: the composing row isn't a call of its own",
			codegraph.Calls, codegraph.Seconds)
	}
	// The answer to "who used codegraph", without the rows.
	if codegraph.Lanes != 2 {
		t.Errorf("codegraph lanes = %d, want the 2 that called it", codegraph.Lanes)
	}
	if len(codegraph.Tools) != 2 {
		t.Fatalf("codegraph tools = %+v, want its two methods", codegraph.Tools)
	}
	if codegraph.Tools[0].Leaf != "codegraph_search" || codegraph.Tools[0].Calls != 2 {
		t.Errorf("first method = %+v, want codegraph_search with 2 calls", codegraph.Tools[0])
	}
	if codegraph.Tools[0].Tool != "mcp__codegraph__codegraph_search" {
		t.Errorf("leaf tool = %q, want the raw name a reader can grep for", codegraph.Tools[0].Tool)
	}
	// Summing the leaves' lane counts would say 3, and a group only saw 2.
	if codegraph.Tools[0].Lanes != 2 || codegraph.Tools[1].Lanes != 1 {
		t.Errorf("method lanes = %d and %d, want 2 and 1", codegraph.Tools[0].Lanes, codegraph.Tools[1].Lanes)
	}

	if read.Calls != 2 || read.Errors != 1 {
		t.Errorf("Read = %d calls with %d failures, want 2 and 1", read.Calls, read.Errors)
	}
	if len(read.Tools) != 1 || read.Tools[0].Leaf != "Read" {
		t.Errorf("Read tools = %+v, want the one leaf", read.Tools)
	}
}

// TestTimelineToolTotalsAgreeWithTheRows holds the breakdown to the rows it's made of, on the derivation's own golden
// fixture rather than on a hand-made timeline.
func TestTimelineToolTotalsAgreeWithTheRows(t *testing.T) {
	got := decode[report.Timeline](t, get(t, goldenRoot(), "/api/sessions/"+goldenID+"/timeline"))

	if len(got.Totals.ByTool) == 0 {
		t.Fatal("the fixture makes tool calls, so the breakdown shouldn't be empty")
	}

	var calls int
	var seconds float64
	for _, group := range got.Totals.ByTool {
		var leafCalls int
		for _, leaf := range group.Tools {
			leafCalls += leaf.Calls
		}
		if leafCalls != group.Calls {
			t.Errorf("group %q says %d calls, its leaves add up to %d", group.Group, group.Calls, leafCalls)
		}
		calls += group.Calls
		seconds += group.Seconds
	}

	// A click on a slice filters the sheet by this, so every row that ran a tool has to name a group that exists.
	groups := map[string]bool{}
	for _, group := range got.Totals.ByTool {
		groups[group.Group] = true
	}

	var wantCalls int
	var wantSeconds float64
	for _, r := range got.Rows {
		if r.Kind == string(timeline.KindToolCall) {
			wantCalls++
		}
		if r.Tool == "" {
			if r.ToolGroup != "" {
				t.Errorf("row %d carries a tool group with no tool: %+v", r.Line, r)
			}
			continue
		}
		if !groups[r.ToolGroup] {
			t.Errorf("row %d is in group %q, which the breakdown doesn't list", r.Line, r.ToolGroup)
		}
		if r.Kind != string(timeline.KindToolCall) {
			wantSeconds += r.Seconds
		}
	}
	if calls != wantCalls {
		t.Errorf("the breakdown counts %d calls, the rows hold %d", calls, wantCalls)
	}
	if math.Abs(seconds-wantSeconds) > 0.01 {
		t.Errorf("the breakdown adds up to %vs, the rows that ran a tool to %vs", seconds, wantSeconds)
	}
}

// TestTimelineGapsAreTheStretchesALaneProducedNothing keeps the swimlane's holes honest: every gap has to be a row of
// the same lane that the derivation called idle.
func TestTimelineGapsAreTheStretchesALaneProducedNothing(t *testing.T) {
	got := decode[report.Timeline](t, get(t, goldenRoot(), "/api/sessions/"+goldenID+"/timeline"))

	gaps := 0
	for _, lane := range got.Lanes {
		for _, gap := range lane.Gaps {
			gaps++
			if !timeline.Kind(gap.Kind).IsGap() {
				t.Errorf("lane %s: gap of kind %q, want the kinds where the lane produced nothing", lane.Name, gap.Kind)
			}
			found := false
			for _, r := range got.Rows {
				if r.LaneID == lane.ID && r.From.Equal(gap.From) && r.Until.Equal(gap.Until) {
					found = true
				}
			}
			if !found {
				t.Errorf("lane %s: gap %s → %s isn't one of its rows", lane.Name, gap.From, gap.Until)
			}
		}
	}
	if gaps == 0 {
		t.Error("the fixture has waits and stalls in it, so the lanes should carry gaps")
	}
}

// TestTimelineCanLeaveTheRowsOut is what a session with 983 lanes needs: the pie and the swimlane without the sheet.
func TestTimelineCanLeaveTheRowsOut(t *testing.T) {
	full := decode[report.Timeline](t, get(t, goldenRoot(), "/api/sessions/"+goldenID+"/timeline"))
	light := decode[report.Timeline](t, get(t, goldenRoot(), "/api/sessions/"+goldenID+"/timeline?rows=false"))

	if len(light.Rows) != 0 {
		t.Errorf("returned %d rows, want none", len(light.Rows))
	}
	if light.Totals.Rows != full.Totals.Rows {
		t.Errorf("totals say %d rows, want the %d it counted", light.Totals.Rows, full.Totals.Rows)
	}
	if len(light.Lanes) != len(full.Lanes) {
		t.Errorf("lanes = %d, want the same %d", len(light.Lanes), len(full.Lanes))
	}
	if light.Totals.LaneTimeSeconds != full.Totals.LaneTimeSeconds {
		t.Errorf("lane time = %v, want the same %v", light.Totals.LaneTimeSeconds, full.Totals.LaneTimeSeconds)
	}
}

func TestAnUnknownPathIs404WithAJSONBody(t *testing.T) {
	rec := get(t, sessionRoot(), "/api/nothing-here")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if code := decode[errorBody](t, rec).Error.Code; code != "not_found" {
		t.Errorf("code = %q", code)
	}
}

func TestTheWrongMethodIs405WithAJSONBody(t *testing.T) {
	rec := httptest.NewRecorder()
	New(Options{Root: sessionRoot()}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/sessions", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if code := decode[errorBody](t, rec).Error.Code; code != "method_not_allowed" {
		t.Errorf("code = %q", code)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET" {
		t.Errorf("Allow = %q, want GET", allow)
	}
}

// TestAPreflightIsAnsweredForTheFrontendAndNobodyElse covers the request a fetch with an unusual header sends first.
// It never reaches a route, so it has to be answered before the mux.
func TestAPreflightIsAnsweredForTheFrontendAndNobodyElse(t *testing.T) {
	handler := New(Options{Root: sessionRoot(), FrontendOrigins: []string{"http://127.0.0.1:19428"}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/sessions", nil)
	req.Header.Set("Origin", "http://127.0.0.1:19428")
	req.Header.Set("Access-Control-Request-Method", "GET")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET" {
		t.Errorf("Access-Control-Allow-Methods = %q, want GET", got)
	}

	other := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodOptions, "/api/sessions", nil)
	req.Header.Set("Origin", "http://evil.example")
	handler.ServeHTTP(other, req)

	if other.Code == http.StatusNoContent {
		t.Error("a preflight from another origin shouldn't be waved through")
	}
}

// TestOnlyTheFrontendOriginCanReadTheAnswer keeps the browser from handing a session's contents to a page that isn't
// this tool's own frontend. The port comes from `.env`, and nothing else is on the list.
func TestOnlyTheFrontendOriginCanReadTheAnswer(t *testing.T) {
	handler := New(Options{Root: sessionRoot(), FrontendOrigins: []string{"http://127.0.0.1:19428"}})

	for _, c := range []struct{ origin, want string }{
		{"http://127.0.0.1:19428", "http://127.0.0.1:19428"},
		{"http://evil.example", ""},
		{"http://127.0.0.1:3000", ""},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
		req.Header.Set("Origin", c.origin)
		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != c.want {
			t.Errorf("origin %s got Access-Control-Allow-Origin %q, want %q", c.origin, got, c.want)
		}
	}
}

// TestATimelineWithNoTimestampsAnswersNullRatherThanYearOne holds the contract in `docs/api.md`: an instant that isn't
// known is null, never a zero date. 99 of the 725 sessions on the machine this was built against carry no timestamped
// record at all, and a frontend that formats a zero date shows a reader "1-01-01 00:53".
func TestATimelineWithNoTimestampsAnswersNullRatherThanYearOne(t *testing.T) {
	const id = "44444444-4444-4444-4444-444444444444"
	root := t.TempDir()
	dir := filepath.Join(root, "-tmp-gamma")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	record := `{"type":"user","uuid":"g1","sessionId":"` + id + `","cwd":"/tmp/gamma","message":{"role":"user","content":"No clock on any of this."}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(record), 0o644); err != nil {
		t.Fatal(err)
	}

	got := decode[report.Timeline](t, get(t, root, "/api/sessions/"+id+"/timeline"))

	if got.Totals.From != nil {
		t.Errorf("totals.from = %v, want null on a session with no timestamped record", got.Totals.From)
	}
	if got.Totals.Until != nil {
		t.Errorf("totals.until = %v, want null on a session with no timestamped record", got.Totals.Until)
	}
	if got.Totals.WallClockSeconds != 0 {
		t.Errorf("wallClockSeconds = %v, want 0", got.Totals.WallClockSeconds)
	}
}
