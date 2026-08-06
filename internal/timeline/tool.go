package timeline

import "github.com/vdavid/claude-session-analyzer/internal/transcript"

// ToolClass is what kind of work a tool call was doing. It's read off the call's own arguments, mostly the shell
// command, because the tool's name alone says almost nothing: nearly every interesting call is a Bash call.
type ToolClass string

const (
	// ClassChecker is the project's own gate: `pnpm check`, `./scripts/check.sh`, `make check`. Running for minutes is
	// its job.
	ClassChecker ToolClass = "checker"
	// ClassBuild is a compiler or bundler.
	ClassBuild ToolClass = "build"
	// ClassTest is a test runner.
	ClassTest ToolClass = "test"
	// ClassDevServer is a long-running process the agent started to work against.
	ClassDevServer ToolClass = "dev server"
	// ClassWait is a call whose whole purpose is to block: a sleep, or a loop polling until something comes up. These
	// are the calls that hit the harness timeout, and they're never a stall.
	ClassWait ToolClass = "wait"
	// ClassGit is version control, including the GitHub CLI.
	ClassGit ToolClass = "git"
	// ClassSearch is grep, find, and friends, plus the search tools.
	ClassSearch ToolClass = "search"
	// ClassFileRead is reading a file or listing a directory.
	ClassFileRead ToolClass = "file read"
	// ClassFileWrite is creating, editing, moving, or removing files.
	ClassFileWrite ToolClass = "file write"
	// ClassAgent is spawning or messaging a teammate.
	ClassAgent ToolClass = "agent"
	// ClassMCP is a call into an MCP server.
	ClassMCP ToolClass = "mcp"
	// ClassWeb is fetching or searching the web.
	ClassWeb ToolClass = "web"
	// ClassShell is a shell command that's none of the above.
	ClassShell ToolClass = "shell"
	// ClassOther is a tool this package doesn't recognise.
	ClassOther ToolClass = "other"
)

// Classify reads a tool call and says what kind of work it was doing.
func Classify(b transcript.Block) ToolClass { return ClassOther }
