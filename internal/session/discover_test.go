package session

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

const (
	alphaID  = "11111111-1111-1111-1111-111111111111"
	soloID   = "22222222-2222-2222-2222-222222222222"
	betaOneD = "33333333-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

func testRoot() string { return filepath.Join("testdata", "projects") }

func TestFindLocatesASessionAndItsSubagentLanes(t *testing.T) {
	loc, err := Find(testRoot(), alphaID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}

	if loc.ID != alphaID {
		t.Errorf("id = %q", loc.ID)
	}
	if loc.ProjectSlug != "-tmp-alpha" {
		t.Errorf("slug = %q", loc.ProjectSlug)
	}
	if want := filepath.Join(testRoot(), "-tmp-alpha", alphaID+".jsonl"); loc.TranscriptPath != want {
		t.Errorf("transcript = %q, want %q", loc.TranscriptPath, want)
	}
	if loc.DirPath == "" {
		t.Error("dir path should be set when the session has a sibling directory")
	}

	want := []string{
		"agent-abuilder-aaaa1111.jsonl",
		"agent-acccc3333.jsonl",
		"agent-alegacy-bbbb2222.jsonl",
	}
	if len(loc.SubagentPaths) != len(want) {
		t.Fatalf("subagent paths = %v, want %d of them", loc.SubagentPaths, len(want))
	}
	for i, w := range want {
		if got := filepath.Base(loc.SubagentPaths[i]); got != w {
			t.Errorf("subagent %d = %q, want %q (sorted by name)", i, got, w)
		}
	}
}

func TestFindHandlesASessionWithNoSubagents(t *testing.T) {
	loc, err := Find(testRoot(), soloID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if loc.DirPath != "" {
		t.Errorf("dir path = %q, want empty", loc.DirPath)
	}
	if len(loc.SubagentPaths) != 0 {
		t.Errorf("subagent paths = %v, want none", loc.SubagentPaths)
	}
}

func TestFindIgnoresWakatimeSiblings(t *testing.T) {
	// The wakatime file sits next to the alpha transcript. Finding by its full name must not work.
	if _, err := Find(testRoot(), alphaID+".jsonl"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestFindAcceptsAUniquePrefix(t *testing.T) {
	loc, err := Find(testRoot(), "1111")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if loc.ID != alphaID {
		t.Errorf("id = %q, want the one session starting with 1111", loc.ID)
	}
}

func TestFindNamesTheCandidatesWhenAPrefixIsAmbiguous(t *testing.T) {
	_, err := Find(testRoot(), "33333333")

	var ambiguous *AmbiguousIDError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("err = %v, want an AmbiguousIDError", err)
	}
	if len(ambiguous.Matches) != 2 {
		t.Fatalf("matches = %v, want 2", ambiguous.Matches)
	}
	msg := err.Error()
	for _, want := range []string{"33333333-aaaa", "33333333-bbbb", "-tmp-beta"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q should name %q so a person can pick", msg, want)
		}
	}
}

func TestFindSaysSoWhenTheIDIsUnknown(t *testing.T) {
	_, err := Find(testRoot(), "no-such-session")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "no-such-session") {
		t.Errorf("message %q should repeat the id that was looked for", err.Error())
	}
}

func TestFindReportsAMissingRoot(t *testing.T) {
	if _, err := Find(filepath.Join(t.TempDir(), "absent"), alphaID); err == nil {
		t.Fatal("want an error for a root that isn't there")
	}
}

func TestDefaultRootFollowsClaudeConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/tmp/somewhere")

	got, err := DefaultRoot()
	if err != nil {
		t.Fatalf("default root: %v", err)
	}
	if want := filepath.Join("/tmp/somewhere", "projects"); got != want {
		t.Errorf("root = %q, want %q", got, want)
	}
}

func TestDefaultRootFallsBackToTheHomeDirectory(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	got, err := DefaultRoot()
	if err != nil {
		t.Fatalf("default root: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join(".claude", "projects")) {
		t.Errorf("root = %q, want it under ~/.claude/projects", got)
	}
}
