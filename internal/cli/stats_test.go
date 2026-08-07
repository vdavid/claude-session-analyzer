package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// goldenRows is what the derivation's golden CSV holds for the fixture session.
const goldenRows = 38

// isolate points the digest cache at a temporary directory, so a test can't read or write the cache on the machine
// running it. `cache.Open` honours XDG_CACHE_HOME, which is the whole reason it does.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
}

func TestStatsSaysWhereOneSessionsTimeWent(t *testing.T) {
	isolate(t)
	code, stdout, stderr := run(t, "stats", goldenID, "--root", goldenRoot(), "--group-by", "kind")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}

	for _, want := range []string{"Kind", "Share of lane time", "lane time", "net", "active", "wall clock"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("wanted %q in the answer:\n%s", want, stdout)
		}
	}
}

// TestStatsSharesAgainstLaneTimeRatherThanActive guards the reading a waiting row invites. Active time excludes
// waiting, so dividing a wait by it once produced "94% of active" for a slice that isn't part of active at all.
func TestStatsSharesAgainstLaneTimeRatherThanActive(t *testing.T) {
	isolate(t)
	_, stdout, _ := run(t, "stats", goldenID, "--root", goldenRoot(), "--group-by", "kind")
	if strings.Contains(stdout, "Share of active") {
		t.Errorf("the group column has to name lane time, the denominator the groups partition:\n%s", stdout)
	}
}

// TestStatsPrintsTheLadderAsRungsRatherThanASentence holds the layout decision. Three durations in one sentence is how a
// reader ends up quoting the wrong one, so each rung gets a line with the subtraction written beside it, and net leads
// because it's the one answering "what did this cost".
func TestStatsPrintsTheLadderAsRungsRatherThanASentence(t *testing.T) {
	isolate(t)
	_, stdout, _ := run(t, "stats", goldenID, "--root", goldenRoot(), "--group-by", "kind")

	lines := strings.Split(stdout, "\n")
	rungs := map[string]bool{}
	for _, line := range lines {
		for _, rung := range []string{"lane time", "net", "active"} {
			if strings.HasPrefix(strings.TrimSpace(line), rung+" ") {
				rungs[rung] = true
			}
		}
	}
	for _, rung := range []string{"lane time", "net", "active"} {
		if !rungs[rung] {
			t.Errorf("no line of its own for the %q rung:\n%s", rung, stdout)
		}
	}
	if !strings.Contains(stdout, "Net agent time") {
		t.Errorf("the summary should lead with net:\n%s", stdout)
	}
	for _, subtraction := range []string{"minus waiting for a person or a teammate", "minus stalls"} {
		if !strings.Contains(stdout, subtraction) {
			t.Errorf("a rung without its subtraction beside it is a number a reader can misquote, wanted %q:\n%s",
				subtraction, stdout)
		}
	}
}

// TestStatsNamesTheClockItsTimeColumnHolds guards the reading a tool question invites. Its time column is the tool
// running, with the agent composing the call and any stall in columns of their own, so a header saying "Time" would
// invite a reader to take it for everything the tool cost.
func TestStatsNamesTheClockItsTimeColumnHolds(t *testing.T) {
	isolate(t)
	_, tools, _ := run(t, "stats", goldenID, "--root", goldenRoot(), "--group-by", "group")
	for _, want := range []string{"Running", "Composing", "Stalled", "Running / lane time"} {
		if !strings.Contains(tools, want) {
			t.Errorf("wanted a %q column on a tool question:\n%s", want, tools)
		}
	}

	// A question that isn't about tools has one clock, and the other two would be the same numbers a second time: the
	// `tool call` row already is the composing time and the `stalled` row already is the stalled time.
	_, kinds, _ := run(t, "stats", goldenID, "--root", goldenRoot(), "--group-by", "kind")
	for _, unwanted := range []string{"Composing", "Stalled", "Running"} {
		if strings.Contains(kinds, unwanted) {
			t.Errorf("a kind breakdown shouldn't carry a %q column, its rows are those numbers:\n%s", unwanted, kinds)
		}
	}
	if !strings.Contains(kinds, "Share of lane time") {
		t.Errorf("a kind breakdown shares against lane time and says so:\n%s", kinds)
	}
}

func TestStatsCountsAToolCallOnceRatherThanTwice(t *testing.T) {
	isolate(t)
	code, stdout, stderr := run(t, "stats", goldenID, "--root", goldenRoot(),
		"--where", "class=shell", "--group-by", "kind", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}

	var body struct {
		Groups []struct {
			Kind string `json:"kind"`
		} `json:"groups"`
		Notes []string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(stdout), &body); err != nil {
		t.Fatalf("the answer isn't JSON: %v\n%s", err, stdout)
	}
	for _, g := range body.Groups {
		if g.Kind == "tool call" {
			t.Errorf("a tool question kept the row where the agent composed the call, which double counts it:\n%s",
				stdout)
		}
	}
	if len(body.Notes) == 0 {
		t.Errorf("a tool question has to say which of a call's three clocks its seconds are:\n%s", stdout)
	}
}

// A corpus answer says how many sessions each group covers, which is the number "codegraph in 12 of 735 sessions"
// needs. Grouping by session leaves the column out, because there it's 1 on every row.
func TestStatsTableCountsSessionsPerGroupExceptWhenGroupingBySession(t *testing.T) {
	isolate(t)
	code, stdout, stderr := run(t, "stats", "--root", sessionRoot(), "--group-by", "project")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Sessions") {
		t.Errorf("wanted a Sessions column:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Calls") {
		t.Errorf("the Calls column has to stay:\n%s", stdout)
	}

	code, bySession, stderr := run(t, "stats", "--root", sessionRoot(), "--group-by", "session")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if strings.Contains(bySession, "Sessions") {
		t.Errorf("a per-session answer counts 1 session on every row, so the column adds nothing:\n%s", bySession)
	}
}

func TestStatsRefusesADimensionItDoesntHaveAndNamesTheOnesItDoes(t *testing.T) {
	isolate(t)
	code, _, stderr := run(t, "stats", "--root", goldenRoot(), "--group-by", "nonsense")
	if code != 2 {
		t.Errorf("exit %d, want 2: a bad dimension is a mistake in the command line", code)
	}
	for _, want := range []string{"nonsense", "kind", "class", "group"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("wanted %q in the refusal, got: %s", want, stderr)
		}
	}
}

func TestStatsListsItsVocabularyWithoutReadingATranscript(t *testing.T) {
	isolate(t)
	code, stdout, stderr := run(t, "stats", "--vocabulary", "--root", "/nowhere-at-all")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"kind", "class", "leaf", "thinking", "waiting for a person", "checker", "mcp"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("wanted %q in the vocabulary:\n%s", want, stdout)
		}
	}
}

func TestSessionsJSONFiltersByDateAndCountsEveryMatchInTheTotals(t *testing.T) {
	code, stdout, stderr := run(t, "sessions", "--root", sessionRoot(), "--json", "--limit", "1")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}

	var body struct {
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
		Totals struct {
			Sessions int `json:"sessions"`
		} `json:"totals"`
	}
	if err := json.Unmarshal([]byte(stdout), &body); err != nil {
		t.Fatalf("the listing isn't JSON: %v\n%s", err, stdout)
	}
	if len(body.Sessions) != 1 {
		t.Errorf("the limit showed %d sessions, want 1", len(body.Sessions))
	}
	// The limit bounds what's shown, never what's counted: "how many sessions in July" is the total, not the array.
	if body.Totals.Sessions <= 1 {
		t.Errorf("totals counted %d, want every session the filters kept", body.Totals.Sessions)
	}
}

func TestSessionsSaysWhichDateItCouldntRead(t *testing.T) {
	code, _, stderr := run(t, "sessions", "--root", sessionRoot(), "--since", "last tuesday")
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr, "last tuesday") || !strings.Contains(stderr, "2026-07-01") {
		t.Errorf("wanted the bad value and the shape of a good one, got: %s", stderr)
	}
}

// TestTimelineJSONLeavesRowsOutUnlessAsked is the bound on the biggest thing this CLI can print. The reference
// session's rows are 8 MB, and handing that to an agent by default is a context window gone.
func TestTimelineJSONLeavesRowsOutUnlessAsked(t *testing.T) {
	code, stdout, stderr := run(t, "timeline", goldenID, "--root", goldenRoot(), "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}

	var body struct {
		Totals struct {
			Rows          int     `json:"rows"`
			ActiveSeconds float64 `json:"activeSeconds"`
		} `json:"totals"`
		Rows []any `json:"rows"`
	}
	if err := json.Unmarshal([]byte(stdout), &body); err != nil {
		t.Fatalf("the timeline isn't JSON: %v", err)
	}
	if len(body.Rows) != 0 {
		t.Errorf("got %d rows, want none without --rows", len(body.Rows))
	}
	if body.Totals.Rows != goldenRows {
		t.Errorf("totals.rows = %d, want %d: leaving rows out mustn't lose the count", body.Totals.Rows, goldenRows)
	}

	_, withRows, _ := run(t, "timeline", goldenID, "--root", goldenRoot(), "--json", "--rows")
	if err := json.Unmarshal([]byte(withRows), &body); err != nil {
		t.Fatalf("the timeline isn't JSON: %v", err)
	}
	if len(body.Rows) != goldenRows {
		t.Errorf("--rows gave %d rows, want %d", len(body.Rows), goldenRows)
	}
}

func TestCacheInfoSaysNothingIsCachedBeforeAnythingIs(t *testing.T) {
	isolate(t)
	code, stdout, stderr := run(t, "cache", "info", "--root", goldenRoot())
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "cache warm") {
		t.Errorf("an empty cache has to say how to fill it, got: %s", stdout)
	}
}

func TestCacheWarmThenInfoReportsWhatItStored(t *testing.T) {
	isolate(t)
	if code, _, stderr := run(t, "cache", "warm", "--root", goldenRoot()); code != 0 {
		t.Fatalf("warm exited %d, stderr: %s", code, stderr)
	}
	code, stdout, stderr := run(t, "cache", "info", "--root", goldenRoot())
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "cached") || !strings.Contains(stdout, "derivation version") {
		t.Errorf("wanted a count and the derivation version, got: %s", stdout)
	}

	// A warmed session answers from the cache rather than the transcripts.
	_, answer, _ := run(t, "stats", goldenID, "--root", goldenRoot(), "--group-by", "kind")
	if !strings.Contains(answer, "1 cached") {
		t.Errorf("wanted the answer to come from the cache, got: %s", answer)
	}
}

func TestCacheTakesOnlyWhatItKnowsHowToDo(t *testing.T) {
	isolate(t)
	code, _, stderr := run(t, "cache", "polish", "--root", goldenRoot())
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr, "warm") || !strings.Contains(stderr, "clear") {
		t.Errorf("wanted the refusal to name what it does take, got: %s", stderr)
	}
}
