package timeline

import (
	"slices"
	"testing"
)

func TestEveryClassHasACategory(t *testing.T) {
	if len(Classes) == 0 {
		t.Fatal("the engine lists no tool classes at all")
	}
	for _, class := range Classes {
		category := CategoryOf(class, "")
		if category == "" {
			t.Errorf("the class %q has no category. Add it to classCategories in category.go, picking the bucket a reader would look for it in rather than letting it default", class)
			continue
		}
		if !slices.Contains(Categories, category) {
			t.Errorf("the class %q maps to %q, which isn't one of the categories: %v", class, category, Categories)
		}
	}
}

// TestACategoryComesFromTheGroupWhereTheClassCantSayIt covers the overrides one at a time, because each one encodes a
// workflow fact no rule can infer: three MCP servers whose class says only "mcp", and two web tools that are research
// rather than QA.
func TestACategoryComesFromTheGroupWhereTheClassCantSayIt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tool  string
		cmd   string
		class ToolClass
		want  ToolCategory
	}{
		{"codegraph is how the agents read a codebase", "mcp__codegraph__codegraph_search", "", ClassMCP, CategoryRead},
		{"tauri drives the app under test", "mcp__tauri__tauri_click", "", ClassMCP, CategoryQA},
		{"chrome-devtools drives a browser against the app", "mcp__chrome-devtools__navigate_page", "", ClassMCP, CategoryQA},
		{"an MCP server nobody named isn't QA", "mcp__google-sheets__get_sheet_data", "", ClassMCP, CategoryOther},
		{"WebFetch is research rather than QA", "WebFetch", "", ClassWeb, CategoryOther},
		{"WebSearch is the same", "WebSearch", "", ClassWeb, CategoryOther},
		{"a curl against the app under test stays QA", "Bash", "curl http://127.0.0.1:19427/api/sessions", ClassWeb, CategoryQA},
		{"a checker run is a check", "Bash", "pnpm check", ClassChecker, CategoryChecks},
		{"a dev server is part of building", "Bash", "pnpm dev", ClassDevServer, CategoryBuild},
		{"git is managing the work", "Bash", "git commit -m fix", ClassGit, CategoryManagement},
		{"spawning a teammate is managing the work", "Agent", "", ClassAgent, CategoryManagement},
		{"a search is reading", "Grep", "", ClassSearch, CategoryRead},
		{"an edit is writing", "Edit", "", ClassFileWrite, CategoryWrite},
		{"a blocking sleep is neither", "Bash", "sleep 30", ClassWait, CategoryOther},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := Identify(toolUse(tc.tool, tc.cmd))
			if id.Class != tc.class {
				t.Fatalf("class: got %q, want %q. The case is about the category, so a wrong class means the fixture moved", id.Class, tc.class)
			}
			if id.Category != tc.want {
				t.Errorf("category of %q (group %q): got %q, want %q", tc.tool, id.Group, id.Category, tc.want)
			}
			if got := CategoryOf(id.Class, id.Group); got != tc.want {
				t.Errorf("CategoryOf disagrees with Identify: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestACellCarryingNoToolHasNoCategory keeps a category off the thinking and waiting rows. A category is a fact about a
// tool call, and defaulting the rest to Other would put most of a session's clock in a bucket nobody asked about.
func TestACellCarryingNoToolHasNoCategory(t *testing.T) {
	if got := CategoryOf("", ""); got != "" {
		t.Errorf("a row with no class got the category %q, and it should get none", got)
	}
}

func TestTheCategoryListCarriesEveryCategoryInOrderWithALabel(t *testing.T) {
	list := CategoryList()
	if len(list) != len(Categories) {
		t.Fatalf("the described list holds %d categories and the order holds %d", len(list), len(Categories))
	}
	for i, meta := range list {
		if meta.Category != Categories[i] {
			t.Errorf("position %d: the described list says %q and the order says %q", i, meta.Category, Categories[i])
		}
		if meta.Label == "" || meta.Description == "" {
			t.Errorf("the category %q has no label or no description, so a legend has nothing to print", meta.Category)
		}
	}
}

// TestTheClassificationProbesCoverEveryClassAndEveryOverride is what keeps the cache's version guard honest. The guard
// hashes what the probes classify as, so a class or an override no probe touches is a rule change the guard can't see.
func TestTheClassificationProbesCoverEveryClassAndEveryOverride(t *testing.T) {
	classes := map[ToolClass]bool{}
	groups := map[string]bool{}
	for _, probe := range classificationProbes {
		id := Identify(toolUse(probe.tool, probe.command))
		classes[id.Class] = true
		groups[id.Group] = true
	}
	for _, class := range Classes {
		if !classes[class] {
			t.Errorf("no probe classifies as %q, so a rule change on it would be invisible to the cache's version guard. Add a representative call to classificationProbes", class)
		}
	}
	for group := range groupCategories {
		if !groups[group] {
			t.Errorf("no probe lands in the group %q, so changing its override would be invisible to the cache's version guard. Add a call that reaches it to classificationProbes", group)
		}
	}
}

func TestTheClassificationFingerprintIsStableAcrossCalls(t *testing.T) {
	if a, b := ClassificationFingerprint(), ClassificationFingerprint(); a != b {
		t.Errorf("the fingerprint isn't deterministic, so hashing it would fail at random:\n%s\n%s", a, b)
	}
}
