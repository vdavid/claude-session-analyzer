package timeline

import "testing"

// TestClassifyCommand reads shell commands the way the timeline does. The cases are the shapes that actually show up
// in transcripts, including the compound ones, because a command is rarely one thing.
func TestClassifyCommand(t *testing.T) {
	cases := []struct {
		command string
		want    ToolClass
	}{
		// A single command is whatever its program is.
		{"ls /tmp", ClassFileRead},
		{"cat notes.md", ClassFileRead},
		{"wc -l main.go", ClassFileRead},
		{"du -sh ~/Library", ClassFileRead},
		{"rm -f /tmp/scratch.db", ClassFileWrite},
		{"mkdir -p build && touch build/.keep", ClassFileWrite},
		{"grep -rn TODO .", ClassSearch},
		{"rg --files-with-matches panic", ClassSearch},
		{"find . -name '*.go'", ClassSearch},
		{"git add -A", ClassGit},
		{"gh run watch", ClassGit},

		// Build and test runners are told apart by the program, not by the subcommand: `cargo check` builds, and
		// `pnpm check` is a project's own gate.
		{"cargo build --release", ClassBuild},
		{"cargo check -p app --lib", ClassBuild},
		{"cargo clippy --all-targets", ClassBuild},
		{"go build ./...", ClassBuild},
		{"tsc --noEmit", ClassBuild},
		{"pnpm build", ClassBuild},
		{"cargo test -p app --lib", ClassTest},
		{"cargo nextest run -p app", ClassTest},
		{"go test ./... -run Timeline", ClassTest},
		{"npx vitest run", ClassTest},
		{"pytest -q", ClassTest},
		{"pnpm test", ClassTest},
		{"pnpm check -q clippy", ClassChecker},
		{"pnpm check --fast", ClassChecker},
		{"npm run check", ClassChecker},
		{"./scripts/check.sh --fast", ClassChecker},
		{"make check", ClassChecker},
		{"pnpm dev", ClassDevServer},
		{"cargo run --bin app", ClassDevServer},

		// Anything whose job is to block is a wait. These are the calls that hit the harness timeout, and calling them
		// stalls would be wrong: they were doing exactly what they were asked to.
		{"sleep 30", ClassWait},
		{`until pgrep -f "target/debug/app" > /dev/null; do sleep 3; done; echo RUNNING`, ClassWait},
		{"while ! nc -z 127.0.0.1 19427; do sleep 1; done", ClassWait},
		{"for i in $(seq 1 60); do if nc -z 127.0.0.1 19427; then break; fi; sleep 1; done", ClassWait},

		// A sleep that isn't a poll doesn't make the whole command a wait: the time went somewhere else.
		{"pkill -f app; sleep 3; pgrep -f app | wc -l", ClassFileRead},

		// A compound command is named after the costliest thing in it.
		{"git add -A && pnpm check -q clippy svelte-check", ClassChecker},
		{"cd apps/desktop && npx vitest run 2>&1 | tail -25", ClassTest},
		{`D="$HOME/state"; rm -f "$D"/*.db 2>/dev/null; ls "$D" | grep -i cache; du -sh "$D"`, ClassFileWrite},

		// A heredoc body is data, not commands, so what's inside it doesn't get a vote.
		{"python3 - <<'PY'\nfor i in range(3):\n    print('sleep')\nPY", ClassShell},

		// Quoting hides separators from the splitter.
		{`echo "a; rm -rf b"`, ClassShell},

		{"", ClassShell},
		{"awk '{print $1}' log.txt", ClassShell},
	}

	for _, c := range cases {
		if got := classifyCommand(c.command); got != c.want {
			t.Errorf("classifyCommand(%q) = %q, want %q", c.command, got, c.want)
		}
	}
}

// TestClassifyTool covers the tools that say what they are by name.
func TestClassifyTool(t *testing.T) {
	cases := []struct {
		block transcriptBlockCase
		want  ToolClass
	}{
		{transcriptBlockCase{"Read", "file_path", "/tmp/a"}, ClassFileRead},
		{transcriptBlockCase{"Write", "file_path", "/tmp/a"}, ClassFileWrite},
		{transcriptBlockCase{"Edit", "file_path", "/tmp/a"}, ClassFileWrite},
		{transcriptBlockCase{"NotebookEdit", "file_path", "/tmp/a"}, ClassFileWrite},
		{transcriptBlockCase{"Glob", "pattern", "**/*.go"}, ClassSearch},
		{transcriptBlockCase{"Grep", "pattern", "panic"}, ClassSearch},
		{transcriptBlockCase{"ToolSearch", "query", "select:Read"}, ClassSearch},
		{transcriptBlockCase{"Agent", "description", "review the plan"}, ClassAgent},
		{transcriptBlockCase{"Task", "description", "review the plan"}, ClassAgent},
		{transcriptBlockCase{"SendMessage", "message", "ping"}, ClassAgent},
		{transcriptBlockCase{"WebFetch", "url", "https://example.com"}, ClassWeb},
		{transcriptBlockCase{"WebSearch", "query", "rust"}, ClassWeb},
		{transcriptBlockCase{"mcp__tauri__webview_execute_js", "script", "1+1"}, ClassMCP},
		{transcriptBlockCase{"Bash", "command", "cargo build"}, ClassBuild},
		{transcriptBlockCase{"EnterWorktree", "branch", "x"}, ClassOther},
	}

	for _, c := range cases {
		b := toolUseBlock("id", c.block.tool, c.block.key, c.block.value)
		if got := Classify(b); got != c.want {
			t.Errorf("Classify(%s) = %q, want %q", c.block.tool, got, c.want)
		}
	}
}

type transcriptBlockCase struct {
	tool  string
	key   string
	value string
}

// TestClassifyBashWithoutItsCommand covers a call whose command was too large to keep. It's still a shell command, and
// guessing at more than that would be inventing.
func TestClassifyBashWithoutItsCommand(t *testing.T) {
	b := toolUseBlock("id", "Bash", "description", "Rewrite the fixture")
	b.InputElided = []string{"command"}

	if got := Classify(b); got != ClassShell {
		t.Errorf("Classify = %q, want %q", got, ClassShell)
	}
}
