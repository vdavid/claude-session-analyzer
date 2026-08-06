package timeline

import (
	"strings"

	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

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

// precedence orders the classes by how much of a command's time they explain, costliest first. A compound command is
// named after the highest class in it, so `git add -A && pnpm check` is a checker run and `cargo build | grep error`
// is a build. The tail of the list is the ordinary utilities, which only get to name a command when nothing else is
// happening in it.
var precedence = []ToolClass{
	ClassWait, ClassDevServer, ClassChecker, ClassBuild, ClassTest, ClassGit,
	ClassFileWrite, ClassSearch, ClassFileRead, ClassShell,
}

var precedenceRank = func() map[ToolClass]int {
	rank := make(map[ToolClass]int, len(precedence))
	for i, c := range precedence {
		rank[c] = len(precedence) - i
	}
	return rank
}()

// toolClasses maps the tools that say what they are by name. Bash isn't here: its class comes from its command.
var toolClasses = map[string]ToolClass{
	"Read":         ClassFileRead,
	"NotebookRead": ClassFileRead,
	"Write":        ClassFileWrite,
	"Edit":         ClassFileWrite,
	"MultiEdit":    ClassFileWrite,
	"NotebookEdit": ClassFileWrite,
	"Glob":         ClassSearch,
	"Grep":         ClassSearch,
	"ToolSearch":   ClassSearch,
	"Agent":        ClassAgent,
	"Task":         ClassAgent,
	"SendMessage":  ClassAgent,
	"TaskCreate":   ClassAgent,
	"TaskUpdate":   ClassAgent,
	"TaskList":     ClassAgent,
	"TaskGet":      ClassAgent,
	"WebFetch":     ClassWeb,
	"WebSearch":    ClassWeb,
	"BashOutput":   ClassShell,
	"KillShell":    ClassShell,
}

// programClasses maps a command's program to its class. A program that means different things by subcommand is
// resolved in classifySegment instead.
var programClasses = map[string]ToolClass{
	"sleep": ClassWait,

	"tsc": ClassBuild, "webpack": ClassBuild, "esbuild": ClassBuild, "rollup": ClassBuild,
	"xcodebuild": ClassBuild, "gcc": ClassBuild, "clang": ClassBuild,
	"javac": ClassBuild, "gradle": ClassBuild, "mvn": ClassBuild,

	"pytest": ClassTest, "vitest": ClassTest, "jest": ClassTest, "nextest": ClassTest,
	"playwright": ClassTest, "phpunit": ClassTest,

	"git": ClassGit, "gh": ClassGit, "hg": ClassGit, "jj": ClassGit,

	"grep": ClassSearch, "rg": ClassSearch, "ag": ClassSearch, "ack": ClassSearch,
	"find": ClassSearch, "fd": ClassSearch, "fzf": ClassSearch,

	"cat": ClassFileRead, "head": ClassFileRead, "tail": ClassFileRead, "less": ClassFileRead,
	"more": ClassFileRead, "wc": ClassFileRead, "ls": ClassFileRead, "stat": ClassFileRead,
	"du": ClassFileRead, "df": ClassFileRead, "file": ClassFileRead, "jq": ClassFileRead,
	"diff": ClassFileRead, "tree": ClassFileRead, "shasum": ClassFileRead,

	"rm": ClassFileWrite, "mv": ClassFileWrite, "cp": ClassFileWrite, "mkdir": ClassFileWrite,
	"rmdir": ClassFileWrite, "touch": ClassFileWrite, "chmod": ClassFileWrite, "chown": ClassFileWrite,
	"ln": ClassFileWrite, "tee": ClassFileWrite, "truncate": ClassFileWrite,
	"tar": ClassFileWrite, "unzip": ClassFileWrite, "zip": ClassFileWrite,

	"curl": ClassWeb, "wget": ClassWeb,
}

// packageRunners are the programs that do whatever their first argument says, so the argument decides the class.
var packageRunners = map[string]bool{
	"pnpm": true, "npm": true, "yarn": true, "bun": true, "npx": true, "make": true, "just": true,
}

// runnerScripts maps a package runner's script name to a class. `check` is a project's own gate by convention, which
// is worth knowing because it's the call that legitimately runs for minutes.
var runnerScripts = map[string]ToolClass{
	"check": ClassChecker, "lint": ClassChecker, "typecheck": ClassChecker, "format": ClassChecker,
	"build": ClassBuild, "compile": ClassBuild, "bundle": ClassBuild,
	"test": ClassTest, "vitest": ClassTest, "jest": ClassTest, "playwright": ClassTest, "e2e": ClassTest,
	"dev": ClassDevServer, "start": ClassDevServer, "serve": ClassDevServer, "preview": ClassDevServer,
}

// toolchainSubcommands maps the subcommand of a language toolchain to a class. `cargo check` compiles, which is why a
// name alone can't be trusted: `pnpm check` is a different animal entirely.
var toolchainSubcommands = map[string]ToolClass{
	"build": ClassBuild, "check": ClassBuild, "clippy": ClassBuild, "fmt": ClassBuild,
	"install": ClassBuild, "generate": ClassBuild, "vet": ClassBuild, "doc": ClassBuild,
	"test": ClassTest, "nextest": ClassTest, "bench": ClassTest,
	"run": ClassDevServer,
}

var toolchains = map[string]bool{"cargo": true, "go": true, "dotnet": true, "swift": true}

// Classify reads a tool call and says what kind of work it was doing.
func Classify(b transcript.Block) ToolClass {
	if class, ok := toolClasses[b.ToolName]; ok {
		return class
	}
	if strings.HasPrefix(b.ToolName, "mcp__") {
		return ClassMCP
	}
	if b.ToolName == "Bash" {
		// A command too large to keep is still a shell command, and guessing at more would be inventing.
		return classifyCommand(inputString(b, "command"))
	}
	return ClassOther
}

// classifyCommand names a shell command after the costliest thing it does.
func classifyCommand(cmd string) ToolClass {
	segments := splitCommand(cmd)
	ctx := cmdContext{polling: isPollingLoop(segments), sole: len(segments) == 1}

	best := ClassShell
	for _, seg := range segments {
		class := classifySegment(seg, ctx)
		if precedenceRank[class] > precedenceRank[best] {
			best = class
		}
	}
	return best
}

// cmdContext is what a segment needs to know about the command it sits in.
type cmdContext struct {
	// polling says the command is built around a loop that blocks until something happens.
	polling bool
	// sole says the command is this one segment and nothing else.
	sole bool
}

// isPollingLoop says the command is built around a loop that blocks until something happens. A bare `sleep` in the
// middle of a command isn't one: it's a pause, and the command's time went elsewhere.
func isPollingLoop(segments []string) bool {
	loop, sleep := false, false
	for _, seg := range segments {
		switch firstWord(seg) {
		case "until", "while", "for":
			loop = true
		case "sleep":
			sleep = true
		}
	}
	return loop && sleep
}

// classifySegment names one piece of a compound command.
func classifySegment(seg string, ctx cmdContext) ToolClass {
	words := commandWords(seg)
	if len(words) == 0 {
		return ClassShell
	}

	program := words[0]
	switch program {
	case "until", "while":
		// The loop's own condition is the blocking construct, whatever it polls.
		if ctx.polling {
			return ClassWait
		}
	case "sleep":
		// A sleep is a wait when it's the whole command or the body of a polling loop. A pause in the middle of a
		// longer command is not: the command's time went somewhere else.
		if ctx.polling || ctx.sole {
			return ClassWait
		}
		return ClassShell
	}

	if class, ok := programClasses[program]; ok {
		return class
	}
	if packageRunners[program] {
		return runnerClass(words)
	}
	if toolchains[program] && len(words) > 1 {
		if class, ok := toolchainSubcommands[words[1]]; ok {
			return class
		}
		return ClassShell
	}
	if isCheckerScript(program) {
		return ClassChecker
	}
	return ClassShell
}

// runnerClass reads what a package runner was asked to run, stepping over the `run` in `npm run check`.
func runnerClass(words []string) ToolClass {
	for _, w := range words[1:] {
		if strings.HasPrefix(w, "-") || w == "run" || w == "exec" {
			continue
		}
		if class, ok := runnerScripts[w]; ok {
			return class
		}
		if class, ok := programClasses[w]; ok {
			return class
		}
		if toolchains[w] {
			return ClassBuild
		}
		return ClassShell
	}
	return ClassShell
}

// isCheckerScript spots a project's own gate invoked by path: `./scripts/check.sh`, `bin/lint`.
func isCheckerScript(program string) bool {
	if !strings.ContainsRune(program, '/') {
		return false
	}
	base := program[strings.LastIndex(program, "/")+1:]
	base = strings.TrimSuffix(strings.TrimSuffix(base, ".sh"), ".bash")
	return runnerScripts[base] == ClassChecker
}

// commandWords strips a segment down to its program and arguments: leading environment assignments, the shell keywords
// that only introduce a command, and grouping punctuation all go.
func commandWords(seg string) []string {
	words := strings.Fields(seg)
	for len(words) > 0 {
		w := strings.TrimLeft(words[0], "({!")
		switch w {
		case "", "do", "then", "else", "elif", "if", "time", "exec", "command", "sudo", "nohup":
			words = words[1:]
			continue
		}
		// `VAR=value cmd` and a bare assignment both start with a name and an equals sign.
		if eq := strings.IndexByte(w, '='); eq > 0 && !strings.ContainsAny(w[:eq], "/\"'$") {
			words = words[1:]
			continue
		}
		words[0] = w
		return words
	}
	return nil
}

func firstWord(seg string) string {
	if words := commandWords(seg); len(words) > 0 {
		return words[0]
	}
	return ""
}

// splitCommand breaks a shell command into the commands it runs, on `;`, `&&`, `||`, `|`, and newlines. It steps over
// quoted text and over heredoc bodies, both of which carry separators that aren't separators: `echo "a; b"` is one
// command, and a Python script fed in on a heredoc isn't shell at all.
func splitCommand(cmd string) []string {
	var (
		out     []string
		current strings.Builder
		quote   byte
	)
	flush := func() {
		if seg := strings.TrimSpace(current.String()); seg != "" {
			out = append(out, seg)
		}
		current.Reset()
	}

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
			current.WriteByte(c)
		case c == '\'' || c == '"':
			quote = c
			current.WriteByte(c)
		case c == '\\' && i+1 < len(cmd):
			i++ // an escaped character can't be a separator
		case c == '<' && i+1 < len(cmd) && cmd[i+1] == '<':
			flush()
			i = skipHeredoc(cmd, i)
		case c == ';' || c == '\n' || c == '|' || c == '&':
			// `&&`, `||`, and a background `&` all separate, and so does a plain pipe.
			if i+1 < len(cmd) && cmd[i+1] == c {
				i++
			}
			flush()
		default:
			current.WriteByte(c)
		}
	}
	flush()
	return out
}

// skipHeredoc walks past a `<<DELIM ... DELIM` body and returns the index of its last byte. The body is data the
// command is fed, not commands, so nothing in it gets a vote on the class.
func skipHeredoc(cmd string, start int) int {
	rest := cmd[start+2:]
	i := 0
	for i < len(rest) && (rest[i] == '-' || rest[i] == ' ') {
		i++
	}
	quote := byte(0)
	if i < len(rest) && (rest[i] == '\'' || rest[i] == '"') {
		quote = rest[i]
		i++
	}
	delimStart := i
	for i < len(rest) && isWordByte(rest[i]) {
		i++
	}
	delim := rest[delimStart:i]
	if delim == "" {
		return start + 1
	}
	if quote != 0 && i < len(rest) && rest[i] == quote {
		i++
	}
	if end := strings.Index(rest[i:], "\n"+delim); end >= 0 {
		return start + 2 + i + end + len(delim)
	}
	return len(cmd) - 1 // an unterminated heredoc runs to the end of the command
}

func isWordByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
