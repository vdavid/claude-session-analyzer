package timeline

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// ToolCategory is the coarse bucket a tool call falls into: seven of them over the sixteen ToolClass values, because a
// breakdown someone reads at a glance can hold seven things and not sixteen.
//
// **This mapping is configuration, not engine truth.** A class is derived from a call's own arguments and is defensible
// from the transcript alone; a category is a judgement about what the work was *for* in one person's workflow. Whether
// a dev server belongs with building or with QA, and whether an MCP server is how you read a codebase or a thing you
// drive under test, are answers about how somebody works. So the two tables below are deliberately a flat, readable
// list that someone who has never opened the rest of this package can change: the classes-to-categories default, and
// the per-group overrides for the cases a class can't express.
//
// Changing either invalidates every cached digest, because a digest stores the category it was built under. `Version`
// in `internal/cache` is what does that, and TestTheDigestVersionMovesWithTheDerivation hashes
// ClassificationFingerprint so a change here fails with the number to bump.
type ToolCategory string

const (
	// CategoryManagement is running the work rather than doing it: teammates, version control, questions to a person.
	CategoryManagement ToolCategory = "management"
	// CategoryRead is finding out what's there.
	CategoryRead ToolCategory = "read"
	// CategoryWrite is changing what's there.
	CategoryWrite ToolCategory = "write"
	// CategoryBuild is turning source into something runnable, and the servers run against it.
	CategoryBuild ToolCategory = "build"
	// CategoryChecks is work that only reports: tests, linters, formatters, the project's own gate.
	CategoryChecks ToolCategory = "checks"
	// CategoryQA is driving the product to see whether it works.
	CategoryQA ToolCategory = "qa"
	// CategoryOther is everything that isn't one of the six, the shell included. It's the honest bucket rather than a
	// failure: a `sleep` and a `WebFetch` genuinely aren't any of the above.
	CategoryOther ToolCategory = "other"
)

// CategoryMeta is one category with the words a legend prints. The label and the description are here rather than in
// the frontend so the taxonomy has one definition; the colour a category is drawn in stays in CSS, because a name is
// data and a hex value is design.
type CategoryMeta struct {
	Category    ToolCategory
	Label       string
	Description string
}

// categoryMeta is the categories in the order a legend shows them, coarse work first and the catch-all last.
//
// **The order is load-bearing for colour.** The frontend assigns its palette slots in this order, and those slots were
// validated for colourblind-safe adjacency as a closed ring (`docs/frontend.md`). Reordering this list changes which
// colours touch, so re-run the validator when you do.
var categoryMeta = []CategoryMeta{
	{CategoryManagement, "Management", "Spawning teammates and messaging them, version control, and putting a question to a person."},
	{CategoryRead, "Read", "Reading files, listing directories, searching, and the tools the agents read a codebase with."},
	{CategoryWrite, "Write", "Creating, editing, moving, and removing files."},
	{CategoryBuild, "Build", "Compilers and bundlers, and the dev servers the agents work against."},
	{CategoryChecks, "Checks", "Tests, linters, formatters, and the project's own gate. The calls that earn their minutes."},
	{CategoryQA, "QA", "Driving the product to see whether it works: a browser, or the app itself."},
	{CategoryOther, "Other", "Shell commands that are none of the above, reading the web, calls whose whole job was to block, and any tool with no category yet."},
}

// Categories is every category in legend order, for a caller that wants the order and nothing else.
var Categories = func() []ToolCategory {
	out := make([]ToolCategory, 0, len(categoryMeta))
	for _, meta := range categoryMeta {
		out = append(out, meta.Category)
	}
	return out
}()

// CategoryList is the categories with their labels, in legend order, so a consumer lays a legend out without
// hardcoding seven strings of its own.
func CategoryList() []CategoryMeta { return append([]CategoryMeta(nil), categoryMeta...) }

// classCategories is the default: every class, and the category it falls in. A class missing from here has no category
// at all, which TestEveryClassHasACategory fails on rather than quietly bucketing it as Other.
var classCategories = map[ToolClass]ToolCategory{
	ClassAgent: CategoryManagement,
	ClassGit:   CategoryManagement,
	ClassAsk:   CategoryManagement,

	ClassFileRead: CategoryRead,
	ClassSearch:   CategoryRead,

	ClassFileWrite: CategoryWrite,

	ClassBuild:     CategoryBuild,
	ClassDevServer: CategoryBuild,

	ClassTest:    CategoryChecks,
	ClassChecker: CategoryChecks,
	ClassLint:    CategoryChecks,

	ClassWeb: CategoryQA,

	// An MCP server this file doesn't name falls to Other rather than to QA. Gmail, Sheets, and Calendar are MCP
	// servers and none of them is QA, so guessing QA for an unknown server would be wrong more often than right.
	ClassMCP: CategoryOther,

	ClassWait:  CategoryOther,
	ClassShell: CategoryOther,
	ClassOther: CategoryOther,
}

// groupCategories overrides the class default for one breakdown group. It's keyed on the group name because that's the
// only place the distinction exists: three MCP servers arrive with the same `mcp` class and mean three different kinds
// of work, and two web tools share `web` with a curl against the app under test.
//
// Each line carries its reason, because the reason is the whole content: nothing about the call itself says which of
// these it is.
var groupCategories = map[string]ToolCategory{
	// How the agents read this codebase, so it belongs with reading rather than with services.
	"codegraph (MCP)": CategoryRead,
	// Driving the app under test.
	"tauri (MCP)": CategoryQA,
	// Driving a browser against the app.
	"chrome-devtools (MCP)": CategoryQA,
	// Research, not QA, though it shares the `web` class with a curl against a dev server.
	"WebFetch": CategoryOther,
	// Same.
	"WebSearch": CategoryOther,
}

// CategoryOf says which category a call falls in. The group wins where it's named, because that's where the workflow
// facts live; otherwise the class decides.
//
// A row carrying no class gets no category. Thinking and waiting rows are most of a session's clock and none of it is
// tool work, so defaulting them to Other would put a session's waiting in a bucket about tools.
func CategoryOf(class ToolClass, group string) ToolCategory {
	if class == "" {
		return ""
	}
	if category, ok := groupCategories[group]; ok {
		return category
	}
	return classCategories[class]
}

// CategoryOverrideGroups is every group name the overrides key on, sorted, for a caller reporting on the taxonomy.
func CategoryOverrideGroups() []string {
	out := make([]string, 0, len(groupCategories))
	for group := range groupCategories {
		out = append(out, group)
	}
	sort.Strings(out)
	return out
}

// classificationProbes is one representative call per class, plus one that reaches each group override. They exist so a
// fingerprint of this package's classification covers every rule, whatever a test fixture happens to hold: the cache's
// version guard hashed only the golden CSV once, and twice missed a rule change the golden had no call for.
//
// TestTheClassificationProbesCoverEveryClassAndEveryOverride fails when a class or an override isn't reached from here.
var classificationProbes = []struct {
	tool    string
	command string
}{
	{"Bash", "pnpm check"},
	{"Bash", "cargo build --release"},
	{"Bash", "cargo clippy --all-targets"},
	{"Bash", "go test ./..."},
	{"Bash", "pnpm dev"},
	{"Bash", "sleep 30"},
	{"Bash", "git commit -m fix"},
	{"Bash", "rg --files-with-matches panic"},
	{"Bash", "cat notes.md"},
	{"Bash", "rm -rf build"},
	{"Bash", "curl http://127.0.0.1:19427/api/sessions"},
	{"Bash", "echo hello"},
	{"Read", ""},
	{"Edit", ""},
	{"Grep", ""},
	{"Agent", ""},
	{"AskUserQuestion", ""},
	{"WebFetch", ""},
	{"WebSearch", ""},
	{"mcp__codegraph__codegraph_search", ""},
	{"mcp__tauri__tauri_click", ""},
	{"mcp__chrome-devtools__navigate_page", ""},
	{"mcp__google-sheets__get_sheet_data", ""},
	{"Telepathy", ""},
}

// ClassificationFingerprint renders every classification decision this package makes as one deterministic block of
// text: each class with its category, each group override, the category order, and what each probe above is identified
// as, end to end.
//
// It exists to be hashed. `internal/cache` stores answers derived under these rules, and a digest built under the old
// ones is invisibly wrong. Hashing the golden CSV alone can't see a rule the fixture has no call for, which has now
// happened twice, so the guard hashes this too.
func ClassificationFingerprint() string {
	var b strings.Builder

	b.WriteString("categories\n")
	for _, meta := range categoryMeta {
		b.WriteString(string(meta.Category) + "\t" + meta.Label + "\n")
	}

	b.WriteString("classes\n")
	for _, class := range Classes {
		b.WriteString(string(class) + "\t" + string(classCategories[class]) + "\n")
	}

	b.WriteString("group overrides\n")
	for _, group := range CategoryOverrideGroups() {
		b.WriteString(group + "\t" + string(groupCategories[group]) + "\n")
	}

	b.WriteString("probes\n")
	for _, probe := range classificationProbes {
		id := Identify(toolUse(probe.tool, probe.command))
		b.WriteString(probe.tool + "\t" + probe.command + "\t" +
			string(id.Class) + "|" + id.Group + "|" + id.Leaf + "|" + string(id.Category) + "\n")
	}
	return b.String()
}

// toolUse builds the block Identify reads: a tool name, plus a shell command for a Bash call. A probe is a call the
// engine asks itself about, so it has to arrive in the same shape a transcript's would.
func toolUse(tool, command string) transcript.Block {
	b := transcript.Block{Type: transcript.BlockToolUse, ToolName: tool}
	if command != "" {
		raw, err := json.Marshal(command)
		if err != nil {
			// A Go string always marshals, so this can't happen; panicking beats returning a block that isn't the
			// call the caller asked about.
			panic(err)
		}
		b.Input = map[string]json.RawMessage{"command": raw}
	}
	return b
}
