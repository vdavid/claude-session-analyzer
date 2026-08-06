package timeline

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// TestRealTimeline derives a session that's actually on this machine and reports where its time went. Hand-built
// fixtures can't catch a rule that's wrong about real data, so run this after touching any of them:
//
//	CSA_REAL_SESSION_ID=<id or unique prefix> go test ./internal/timeline -run RealTimeline -v
//
// Set CSA_REAL_ROOT to read from somewhere other than the default transcript root. It checks the tiling property lane
// by lane, and prints the breakdown, the longest rows, and every stall, which is how a threshold gets argued with.
func TestRealTimeline(t *testing.T) {
	tl, s := deriveReal(t)

	t.Logf("session %s: %s", s.ID, s.Title)
	t.Logf("%d lanes, %d rows, %s of wall clock (%s to %s)",
		len(tl.Lanes), len(tl.Rows), FormatDuration(tl.Duration()),
		tl.First.Format(time.RFC3339), tl.Last.Format(time.RFC3339))

	totals := tl.TotalsByKind()
	var sum time.Duration
	for _, d := range totals {
		sum += d
	}
	t.Log("where the time went, summed across lanes running in parallel:")
	for _, kind := range Kinds {
		d := totals[kind]
		t.Logf("  %-16s %12s  %5.1f%%", kind, FormatDuration(d), 100*d.Seconds()/sum.Seconds())
	}
	t.Logf("  %-16s %12s", "total", FormatDuration(sum))

	byLane := map[string]time.Duration{}
	for _, r := range tl.Rows {
		byLane[r.Agent] += r.Duration()
	}
	t.Log("per lane, alive for and busy with:")
	for _, lane := range tl.Lanes {
		busy := byLane[lane.Name] - laneWaitTotal(tl, lane.ID)
		t.Logf("  %-24s alive %10s, working %10s, %d rows",
			lane.Name, FormatDuration(lane.Duration()), FormatDuration(busy), lane.Rows)
	}

	for _, lane := range tl.Lanes {
		var rows []Row
		for _, r := range tl.Rows {
			if r.LaneID == lane.ID {
				rows = append(rows, r)
			}
		}
		if len(rows) == 0 {
			continue
		}
		t.Run("tiling/"+lane.Name, func(t *testing.T) { checkTiling(t, rows, lane.First, lane.Last) })
	}

	t.Log("every stalled row:")
	for _, r := range tl.Rows {
		if r.Kind == KindStalled {
			t.Logf("  %-24s %10s  %s", r.Agent, FormatDuration(r.Duration()), clip(r.Info, 150))
		}
	}

	t.Log("the ten longest rows:")
	longest := append([]Row(nil), tl.Rows...)
	sort.Slice(longest, func(i, j int) bool { return longest[i].Duration() > longest[j].Duration() })
	for _, r := range longest[:min(10, len(longest))] {
		t.Logf("  %-16s %-24s %10s  %s", r.Kind, r.Agent, FormatDuration(r.Duration()), clip(r.Info, 120))
	}

	t.Logf("timed-out calls: %d", countIf(tl, func(r Row) bool { return r.TimedOut }))
}

// TestRealAnomalies holds the derivation to the two events that made this tool worth building, both of them in the
// reference session's `m7-keep-running` lane: wait loops that hit the ten-minute cap over and over, and a `rm` whose
// result came back six hours later. Rules that don't surface both are wrong rules.
//
//	CSA_REAL_SESSION_ID=532ac591 go test ./internal/timeline -run RealAnomalies -v
//
// Set CSA_REAL_LANE to check a different lane, and CSA_REAL_STALL to expect a different stall length.
func TestRealAnomalies(t *testing.T) {
	tl, _ := deriveReal(t)

	lane := os.Getenv("CSA_REAL_LANE")
	if lane == "" {
		lane = "m7-keep-running"
	}
	wantStall := 6*time.Hour + 15*time.Minute
	if raw := os.Getenv("CSA_REAL_STALL"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			t.Fatalf("CSA_REAL_STALL: %v", err)
		}
		wantStall = d
	}

	var timeouts, stalls []Row
	for _, r := range tl.Rows {
		if r.Agent != lane {
			continue
		}
		if r.TimedOut {
			timeouts = append(timeouts, r)
		}
		if r.Kind == KindStalled {
			stalls = append(stalls, r)
		}
	}

	if len(timeouts) < 2 {
		t.Errorf("lane %s should show repeated wait-loop timeouts, found %d", lane, len(timeouts))
	}
	for _, r := range timeouts {
		t.Logf("timed out after %s: %s", FormatDuration(r.Duration()), clip(r.Info, 140))
	}

	if len(stalls) == 0 {
		t.Fatalf("lane %s should show a stall of about %s, found none", lane, FormatDuration(wantStall))
	}
	var longest Row
	for _, r := range stalls {
		t.Logf("stalled for %s: %s", FormatDuration(r.Duration()), clip(r.Info, 140))
		if r.Duration() > longest.Duration() {
			longest = r
		}
	}
	if diff := longest.Duration() - wantStall; diff > 15*time.Minute || diff < -15*time.Minute {
		t.Errorf("the longest stall lasted %s, want about %s", FormatDuration(longest.Duration()),
			FormatDuration(wantStall))
	}
}

// TestRealTimelineSweep derives every session on this machine, which is how a rule that only holds for one session
// gets caught. It's the derivation's half of the parser's corpus sweep:
//
//	CSA_SWEEP=1 go test ./internal/timeline -run RealTimelineSweep -v -timeout 30m
func TestRealTimelineSweep(t *testing.T) {
	if os.Getenv("CSA_SWEEP") == "" {
		t.Skip("set CSA_SWEEP=1 to derive every session on this machine")
	}
	root := realRoot(t)

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}

	var sessions, lanes, rows int
	kinds := map[Kind]int{}
	stalls := map[ToolClass]int{}
	var stalled []string
	longest := map[Kind]Row{}
	longestIn := map[Kind]string{}
	start := time.Now()

	for _, slug := range entries {
		if !slug.IsDir() {
			continue
		}
		files, err := os.ReadDir(root + "/" + slug.Name())
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
				continue
			}
			id := strings.TrimSuffix(file.Name(), ".jsonl")
			loc, err := session.Find(root, id)
			if err != nil {
				t.Errorf("find %s: %v", id, err)
				continue
			}
			s, err := session.Load(loc, transcript.Options{})
			if err != nil {
				t.Errorf("load %s: %v", id, err)
				continue
			}
			tl := Derive(s, Options{})
			sessions++
			lanes += len(tl.Lanes)
			rows += len(tl.Rows)
			for _, r := range tl.Rows {
				kinds[r.Kind]++
				if r.Duration() > longest[r.Kind].Duration() {
					longest[r.Kind], longestIn[r.Kind] = r, id[:8]
				}
				if r.Kind == KindStalled {
					stalls[r.Class]++
					stalled = append(stalled, fmt.Sprintf("%s %s %s: %s",
						id[:8], r.Agent, FormatDuration(r.Duration()), clip(r.Info, 160)))
				}
			}
			checkSweepTiling(t, id, tl)
		}
	}

	t.Logf("%d sessions, %d lanes, %d rows in %s", sessions, lanes, rows, time.Since(start).Round(time.Second))
	for _, kind := range Kinds {
		t.Logf("  %-16s %8d rows", kind, kinds[kind])
	}
	t.Log("stalls by class, which is where a threshold that defames honest work shows up:")
	for class, n := range stalls {
		t.Logf("  %-16s %8d", class, n)
	}
	t.Log("the longest row of each kind, which is where a rule that mislabels a long gap shows up:")
	for _, kind := range Kinds {
		if r, ok := longest[kind]; ok {
			t.Logf("  %-16s %10s  %s %-20s %s", kind, FormatDuration(r.Duration()), longestIn[kind], r.Agent,
				clip(r.Info, 110))
		}
	}
	t.Log("every stall in the corpus, so a wrong call is visible rather than buried in a count:")
	for _, line := range stalled {
		t.Logf("  %s", line)
	}
}

// checkSweepTiling is the tiling check without the per-lane subtests, so a sweep over thousands of sessions reports
// one failure per lane rather than a subtest tree.
func checkSweepTiling(t *testing.T, id string, tl *Timeline) {
	t.Helper()
	for _, lane := range tl.Lanes {
		var prev Row
		var seen bool
		for _, r := range tl.Rows {
			if r.LaneID != lane.ID || r.Overlapped {
				continue
			}
			if r.Duration() < 0 {
				t.Errorf("%s lane %s: negative duration: %s", id, lane.Name, rowSummary(r))
			}
			if seen && !r.From.Equal(prev.Until) {
				t.Errorf("%s lane %s: gap between %s and %s", id, lane.Name, rowSummary(prev), rowSummary(r))
			}
			prev, seen = r, true
		}
		if seen && !prev.Until.Equal(lane.Last) {
			t.Errorf("%s lane %s: rows end at %s, lane ends at %s", id, lane.Name, prev.Until, lane.Last)
		}
	}
}

func deriveReal(t *testing.T) (*Timeline, *session.Session) {
	t.Helper()
	id := os.Getenv("CSA_REAL_SESSION_ID")
	if id == "" {
		t.Skip("set CSA_REAL_SESSION_ID to derive a session on this machine")
	}

	loc, err := session.Find(realRoot(t), id)
	if err != nil {
		t.Fatalf("find %q: %v", id, err)
	}
	s, err := session.Load(loc, transcript.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return Derive(s, Options{}), s
}

func realRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("CSA_REAL_ROOT"); root != "" {
		return root
	}
	root, err := session.DefaultRoot()
	if err != nil {
		t.Fatalf("default root: %v", err)
	}
	return root
}

// laneWaitTotal is how long one lane spent idle, whatever it was idle on.
func laneWaitTotal(tl *Timeline, laneID string) time.Duration {
	var total time.Duration
	for _, r := range tl.Rows {
		if r.LaneID == laneID && r.Kind.IsWaiting() {
			total += r.Duration()
		}
	}
	return total
}

func countIf(tl *Timeline, keep func(Row) bool) int {
	n := 0
	for _, r := range tl.Rows {
		if keep(r) {
			n++
		}
	}
	return n
}
