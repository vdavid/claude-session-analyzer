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
		{"curl -s https://example.com", ClassWeb},
		// A fetch explains a command's time better than reading what it fetched does.
		{"curl -s https://example.com | jq .name", ClassWeb},

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

// TestIdentify covers the two names a breakdown groups a call by. The cases that matter are the two tools that are
// really many: `Bash`, which is 62% of the calls in the corpus, and an MCP server, whose methods would otherwise be
// as many separate tools as it has methods.
func TestIdentify(t *testing.T) {
	cases := []struct {
		block transcriptBlockCase
		want  ToolID
	}{
		// A tool that means one thing is its own group, and its own leaf.
		{transcriptBlockCase{"Read", "file_path", "/tmp/a"}, ToolID{ClassFileRead, "Read", "Read"}},
		{transcriptBlockCase{"Edit", "file_path", "/tmp/a"}, ToolID{ClassFileWrite, "Edit", "Edit"}},
		{transcriptBlockCase{"EnterWorktree", "branch", "x"}, ToolID{ClassOther, "EnterWorktree", "EnterWorktree"}},

		// An MCP call is grouped by the server it went to, and named by the method it called.
		{
			transcriptBlockCase{"mcp__codegraph__codegraph_search", "query", "Row"},
			ToolID{ClassMCP, "codegraph (MCP)", "codegraph_search"},
		},
		{
			// A server name carrying single underscores survives: the separator is two.
			transcriptBlockCase{"mcp__claude_ai_Gmail__get_thread", "query", "x"},
			ToolID{ClassMCP, "claude_ai_Gmail (MCP)", "get_thread"},
		},
		{
			// A name with no method to it still says which server it went to.
			transcriptBlockCase{"mcp__db", "query", "x"},
			ToolID{ClassMCP, "db (MCP)", "db"},
		},

		// Bash is grouped by what the command was doing, and named by the program that earned it.
		{transcriptBlockCase{"Bash", "command", "ls -la /tmp"}, ToolID{ClassFileRead, "Bash (file read)", "ls"}},
		{transcriptBlockCase{"Bash", "command", "git commit -m 'x'"}, ToolID{ClassGit, "Bash (git)", "git commit"}},
		{transcriptBlockCase{"Bash", "command", "gh run watch"}, ToolID{ClassGit, "Bash (git)", "gh run"}},
		{transcriptBlockCase{"Bash", "command", "cargo test -p app"}, ToolID{ClassTest, "Bash (test)", "cargo test"}},
		{transcriptBlockCase{"Bash", "command", "go build ./..."}, ToolID{ClassBuild, "Bash (build)", "go build"}},
		{transcriptBlockCase{"Bash", "command", "pnpm check -q go"}, ToolID{ClassChecker, "Bash (checker)", "pnpm check"}},
		// A runner's `run` is punctuation, so the leaf names what it was asked to run.
		{transcriptBlockCase{"Bash", "command", "npm run check"}, ToolID{ClassChecker, "Bash (checker)", "npm check"}},
		// A gate invoked by path is named by its file, not by the path to it.
		{
			transcriptBlockCase{"Bash", "command", "./scripts/check.sh --fast"},
			ToolID{ClassChecker, "Bash (checker)", "check.sh"},
		},
		// The program that named the compound command is the one the leaf carries.
		{
			transcriptBlockCase{"Bash", "command", "git add -A && pnpm check"},
			ToolID{ClassChecker, "Bash (checker)", "pnpm check"},
		},
		// Nothing in the command outranks a plain shell program, so the first one names it.
		{transcriptBlockCase{"Bash", "command", "awk '{print $1}' log.txt"}, ToolID{ClassShell, "Bash (shell)", "awk"}},
		// Arranging the shell isn't work, so it doesn't get to name the command.
		{
			transcriptBlockCase{"Bash", "command", "cd apps/desktop && python3 tool.py"},
			ToolID{ClassShell, "Bash (shell)", "python3"},
		},
		{
			transcriptBlockCase{"Bash", "command", "export RUST_LOG=debug; python3 tool.py"},
			ToolID{ClassShell, "Bash (shell)", "python3"},
		},
		// A command that only arranges the shell still has to be named after something.
		{transcriptBlockCase{"Bash", "command", "cd /tmp/work"}, ToolID{ClassShell, "Bash (shell)", "cd"}},
		// A wrapper that hides the command behind a duration loses both the class and the program if it's read as one.
		{
			transcriptBlockCase{"Bash", "command", "timeout 120 cargo test -p app"},
			ToolID{ClassTest, "Bash (test)", "cargo test"},
		},
		{
			transcriptBlockCase{"Bash", "command", "timeout -k 5 30s pnpm check"},
			ToolID{ClassChecker, "Bash (checker)", "pnpm check"},
		},
		// Fetching over the network is web work, wherever it sits in the command.
		{
			transcriptBlockCase{"Bash", "command", "curl -s https://example.com | jq .name"},
			ToolID{ClassWeb, "Bash (web)", "curl"},
		},
	}

	for _, c := range cases {
		b := toolUseBlock("id", c.block.tool, c.block.key, c.block.value)
		if got := Identify(b); got != c.want {
			t.Errorf("Identify(%s) = %+v, want %+v", c.block.tool, got, c.want)
		}
	}
}

// TestIdentifyBashWithoutItsCommand covers a call whose command was too large to keep. There's no program to name, and
// inventing one would be worse than saying so.
func TestIdentifyBashWithoutItsCommand(t *testing.T) {
	b := toolUseBlock("id", "Bash", "description", "Rewrite the fixture")
	b.InputElided = []string{"command"}

	want := ToolID{ClassShell, "Bash (shell)", "Bash"}
	if got := Identify(b); got != want {
		t.Errorf("Identify = %+v, want %+v", got, want)
	}
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
