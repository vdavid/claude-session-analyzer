// Package cli is the command line surface over the engine. It parses arguments, resolves a session id, and renders
// what the engine returns. Anything that involves a judgement call belongs in the engine, not here.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// binary is what the tool is called on disk, used in the messages that tell someone what to run next.
const binary = "claude-session-analyzer"

// command is one subcommand. Adding `stats` or `tokens` later means adding an entry here and a run function, which is
// why the CLI has this shape from the start.
type command struct {
	name    string
	summary string
	// usage is the argument line shown after the command name.
	usage string
	run   func(a *app, args []string) error
}

func commands() []command {
	return []command{
		{
			name:    "sessions",
			summary: "List the sessions on disk, newest first",
			run:     runSessions,
		},
		{
			name:    "timeline",
			summary: "Write a CSV timeline for one session",
			usage:   "<session-id>",
			run:     runTimeline,
		},
		{
			name:    "stats",
			summary: "Ask where the time went, over one session or all of them",
			usage:   "[session-id]",
			run:     runStats,
		},
		{
			name:    "cache",
			summary: "Warm, inspect, or clear the digest cache a corpus query reads",
			usage:   "warm|info|clear",
			run:     runCache,
		},
		{
			name:    "serve",
			summary: "Serve the API on 127.0.0.1, for the web app",
			run:     runServe,
		},
	}
}

// app is where a command writes. Holding the two writers rather than reaching for os.Stdout is what makes the whole
// surface testable.
type app struct {
	out io.Writer
	err io.Writer
}

// usageError is a mistake in how the command was called, as opposed to something going wrong while it ran. It exits 2,
// so a script can tell "you asked wrong" from "it didn't work".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

// Run executes one command line and returns the exit code: 0 for success, 1 for a command that couldn't do its job,
// and 2 for a command line that didn't make sense.
func Run(args []string, stdout, stderr io.Writer) int {
	a := &app{out: stdout, err: stderr}

	if len(args) == 0 {
		writeUsage(stderr)
		return 2
	}

	switch args[0] {
	case "help", "-h", "--help":
		writeUsage(stdout)
		return 0
	}

	for _, cmd := range commands() {
		if cmd.name != args[0] {
			continue
		}
		switch err := cmd.run(a, args[1:]); {
		case err == nil:
			return 0
		case errors.Is(err, flag.ErrHelp):
			return 0 // the flag package has already written the command's flags
		default:
			var ue *usageError
			if errors.As(err, &ue) {
				fmt.Fprintln(stderr, ue.msg)
				return 2
			}
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	fmt.Fprintf(stderr, "There's no %q command.\n\n", args[0])
	writeUsage(stderr)
	return 2
}

func writeUsage(w io.Writer) {
	fmt.Fprintf(w, "%s reconstructs where a Claude Code session's time went.\n\n", binary)
	fmt.Fprintf(w, "Usage:\n  %s <command> [flags]\n\nCommands:\n", binary)

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	for _, cmd := range commands() {
		fmt.Fprintf(tw, "  %s\t%s\n", strings.TrimSpace(cmd.name+" "+cmd.usage), cmd.summary)
	}
	_ = tw.Flush()

	fmt.Fprintf(w, "\nRun `%s <command> --help` for a command's flags.\n", binary)
}

// parseArgs parses a command's flags and hands back its positional arguments.
//
// The flag package stops at the first argument that isn't a flag, which would make `timeline <id> --out file.csv` fail
// on a tool whose most natural argument order puts the session id first. So this parses, sets the argument it stopped
// at aside, and carries on with the rest.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string

	// Everything after a bare `--` is an argument, whatever it looks like.
	for i, arg := range args {
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			args = args[:i]
			break
		}
	}

	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// newFlagSet gives a command its flags, writing anything the flag package has to say to stderr and leaving the exit
// code to Run rather than calling os.Exit from under a test.
func newFlagSet(a *app, name string) *flag.FlagSet {
	var cmd command
	for _, c := range commands() {
		if c.name == name {
			cmd = c
		}
	}

	fs := flag.NewFlagSet(binary+" "+name, flag.ContinueOnError)
	fs.SetOutput(a.err)
	fs.Usage = func() {
		fmt.Fprintf(a.err, "%s\n\nUsage:\n  %s %s [flags]\n\nFlags:\n",
			cmd.summary, binary, strings.TrimSpace(name+" "+cmd.usage))
		fs.PrintDefaults()
	}
	return fs
}
