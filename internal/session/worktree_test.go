package session

import (
	"path/filepath"
	"testing"

	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// The worktree fixture holds one session whose lead transcript sits under `-tmp-gamma` while its subagent directory was
// written under the worktree's slug. One lane is split across both, which is the shape that would otherwise be counted
// twice or read out of order.
const worktreeID = "44444444-4444-4444-4444-444444444444"

func worktreeRoot() string { return filepath.Join("testdata", "worktree", "projects") }

func TestFindFollowsASessionIntoTheSlugsItsLanesWereWrittenUnder(t *testing.T) {
	loc, err := Find(worktreeRoot(), worktreeID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}

	if loc.ProjectSlug != "-tmp-gamma" {
		t.Errorf("slug = %q, want the one holding the lead transcript", loc.ProjectSlug)
	}
	if len(loc.DirPaths) != 2 {
		t.Errorf("dir paths = %v, want both the lead's own directory and the worktree's", loc.DirPaths)
	}

	// One lane per agent, whichever slug it landed under, and the workflow's lane after the direct ones.
	want := []string{
		"subagents/agent-ahome-1111.jsonl",
		"subagents/agent-away-2222.jsonl",
		"subagents/workflows/wf_gamma/agent-aworker-3333.jsonl",
	}
	if len(loc.SubagentLanes) != len(want) {
		t.Fatalf("lanes = %v, want %d of them", laneRels(loc), len(want))
	}
	for i, w := range want {
		if got := filepath.ToSlash(loc.SubagentLanes[i].Rel); got != w {
			t.Errorf("lane %d = %q, want %q", i, got, w)
		}
	}
}

func TestFindGathersALaneWrittenUnderTwoSlugsIntoOne(t *testing.T) {
	loc, err := Find(worktreeRoot(), worktreeID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}

	var split LaneFiles
	for _, lane := range loc.SubagentLanes {
		if filepath.Base(lane.Rel) == "agent-ahome-1111.jsonl" {
			split = lane
		}
	}
	if len(split.Paths) != 2 {
		t.Fatalf("the split lane holds %v, want the fragment under each slug", split.Paths)
	}

	// Every path has to be distinct: the same file listed twice is the double count this guards against.
	seen := map[string]bool{}
	for _, lane := range loc.SubagentLanes {
		for _, path := range lane.Paths {
			if seen[path] {
				t.Errorf("%s is listed twice, so its records would be counted twice", path)
			}
			seen[path] = true
		}
	}
}

func TestLoadMergesALaneSplitAcrossSlugsIntoOneOrderedLane(t *testing.T) {
	loc, err := Find(worktreeRoot(), worktreeID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	s, err := Load(loc, transcript.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(s.Lanes) != 4 {
		t.Fatalf("lanes = %d, want the lead plus three subagents: %v", len(s.Lanes), laneNames(s))
	}

	var split *Lane
	for _, lane := range s.Lanes {
		if lane.ID == "ahome-1111" {
			if split != nil {
				t.Fatal("the split lane arrived twice, so its time would be counted twice")
			}
			split = lane
		}
	}
	if split == nil {
		t.Fatalf("no lane for the split agent: %v", laneNames(s))
	}

	// The fragments interleave in time, so concatenating them in slug order would put the records out of order. The
	// chain the records carry says what the right order is: h1 → h2 → h3 → h4.
	var got []string
	for _, rec := range split.Records {
		got = append(got, rec.UUID)
	}
	want := []string{"h1", "h2", "h3", "h4"}
	if len(got) != len(want) {
		t.Fatalf("records = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("records = %v, want %v (every record's parent has to come before it)", got, want)
		}
	}

	if split.Stats.Lines != 4 {
		t.Errorf("stats count %d lines, want all four across both fragments", split.Stats.Lines)
	}
}

// TestLoadReadsMetadataFromWhicheverFragmentCarriesIt covers a lane whose `.meta.json` was written under one slug while
// the records carry on under another.
func TestLoadReadsMetadataFromWhicheverFragmentCarriesIt(t *testing.T) {
	loc, err := Find(worktreeRoot(), worktreeID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	s, err := Load(loc, transcript.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	for _, lane := range s.Lanes {
		if lane.ID != "away-2222" {
			continue
		}
		if lane.Name != "away" {
			t.Errorf("name = %q, want the metadata's name", lane.Name)
		}
		if lane.Meta.Model != "claude-opus-5" {
			t.Errorf("model = %q, want the metadata's", lane.Meta.Model)
		}
		return
	}
	t.Errorf("no lane for the worktree-only agent: %v", laneNames(s))
}

func TestLoadKeepsAWorkflowLaneWrittenUnderAnotherSlug(t *testing.T) {
	loc, err := Find(worktreeRoot(), worktreeID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	s, err := Load(loc, transcript.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	for _, lane := range s.Lanes {
		if lane.ID == "aworker-3333" {
			if lane.WorkflowID != "wf_gamma" {
				t.Errorf("workflow = %q, want wf_gamma", lane.WorkflowID)
			}
			return
		}
	}
	t.Errorf("the workflow's lane is missing: %v", laneNames(s))
}

func TestListCountsLanesWrittenUnderAnotherSlug(t *testing.T) {
	got, err := List(worktreeRoot())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("listed %d sessions, want the one in the fixture: %v", len(got), ids(got))
	}

	s := got[0]
	if s.ID != worktreeID {
		t.Fatalf("id = %q", s.ID)
	}
	if s.Subagents != 3 {
		t.Errorf("subagents = %d, want the three lanes, wherever they were written", s.Subagents)
	}

	// The listing's count has to agree with what a full parse finds, which is what the corpus cross-check does.
	loc, err := Find(worktreeRoot(), worktreeID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	full, err := Load(loc, transcript.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if want := len(full.Lanes) - 1; s.Subagents != want {
		t.Errorf("listing counted %d subagents, a full parse found %d", s.Subagents, want)
	}
}

func laneRels(loc Location) []string {
	out := make([]string, 0, len(loc.SubagentLanes))
	for _, lane := range loc.SubagentLanes {
		out = append(out, lane.Rel)
	}
	return out
}

func laneNames(s *Session) []string {
	out := make([]string, 0, len(s.Lanes))
	for _, lane := range s.Lanes {
		out = append(out, lane.Name)
	}
	return out
}
