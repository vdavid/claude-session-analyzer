package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/report"
	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

// defaultLimit keeps the listing to a screen. There are 722 sessions on the machine this was built against, and the
// one a person is after is nearly always a recent one.
const defaultLimit = 20

// titleWidth and projectWidth keep two columns that can run to any length from pushing everything else off the screen.
// A worktree path is the long one: `~/projects-git/vdavid/cmdr/.claude/worktrees/some-branch-name`.
const (
	titleWidth   = 44
	projectWidth = 40
)

func runSessions(a *app, args []string) error {
	fs := newFlagSet(a, "sessions")
	root := fs.String("root", "", "read transcripts from this directory instead of ~/.claude/projects")
	limit := fs.Int("limit", defaultLimit, "show at most this many sessions, or 0 for all of them")
	asJSON := fs.Bool("json", false, "write the listing as JSON instead of a table")
	var only scope
	only.register(fs)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("`%s sessions` doesn't take arguments, and it got %q.", binary, rest[0])
	}
	keep, err := only.filter()
	if err != nil {
		return err
	}

	dir, err := transcriptRoot(*root)
	if err != nil {
		return err
	}
	all, err := session.List(dir)
	if err != nil {
		if problem := missingRoot(dir, err); problem != nil {
			return problem
		}
		return fmt.Errorf("Couldn't read the transcripts in %s: %w", dir, err)
	}

	sums := make([]session.Summary, 0, len(all))
	for _, s := range all {
		if keep(s) {
			sums = append(sums, s)
		}
	}

	if *asJSON {
		return writeSessionsJSON(a, dir, sums, *limit)
	}
	if len(sums) == 0 {
		if len(all) > 0 {
			fmt.Fprintf(a.out, "None of the %s sessions under %s match those filters.\n", count(len(all)), dir)
			return nil
		}
		fmt.Fprintf(a.out, "No sessions under %s yet.\n", dir)
		return nil
	}

	shown := sums
	if *limit > 0 && *limit < len(shown) {
		shown = shown[:*limit]
	}

	tw := tabwriter.NewWriter(a.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Session\tStarted\tLength\tSubagents\tSize\tProject\tTitle")
	for _, s := range shown {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			s.ID, localTime(s.Start), length(s), count(s.Subagents), humanBytes(s.Bytes),
			clipStart(shortenHome(s.ProjectPath), projectWidth), clip(s.Title, titleWidth))
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("Couldn't write the listing: %w", err)
	}

	var subagents int
	var bytes int64
	for _, s := range sums {
		subagents += s.Subagents
		bytes += s.Bytes
	}
	if len(shown) < len(sums) {
		fmt.Fprintf(a.out, "\nShowing %s of %s sessions (%s subagents, %s on disk). Use `--limit 0` for all of them.\n",
			count(len(shown)), count(len(sums)), count(subagents), humanBytes(bytes))
		return nil
	}
	fmt.Fprintf(a.out, "\n%s sessions, %s subagents, %s on disk. Times are local.\n",
		count(len(sums)), count(subagents), humanBytes(bytes))
	return nil
}

// writeSessionsJSON answers with the same shape the HTTP API's `/api/sessions` does, so a caller learns one
// vocabulary. The totals cover every session the filters kept, not only the ones a limit showed, which is what makes
// "how many sessions did I have in July" a field rather than a count of the array.
func writeSessionsJSON(a *app, dir string, sums []session.Summary, limit int) error {
	body := report.SessionList{Root: dir, Sessions: []report.Session{}}
	for _, s := range sums {
		body.Totals.Sessions++
		body.Totals.Subagents += s.Subagents
		body.Totals.Bytes += s.Bytes
	}

	shown := sums
	if limit > 0 && limit < len(shown) {
		shown = shown[:limit]
	}
	for _, s := range shown {
		body.Sessions = append(body.Sessions, report.ForSession(s))
	}
	return writeJSON(a.out, body)
}

// length is how long a session ran, blank for one whose records carry no timestamps.
func length(s session.Summary) string {
	if s.Start.IsZero() || s.End.IsZero() {
		return ""
	}
	return timeline.FormatDuration(s.Duration())
}

// localTime shows an instant the way the person reading it thinks about it. The transcripts are stamped in UTC and the
// CSV keeps them that way; a listing is for a person.
func localTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Local().Format("2006-01-02 15:04")
}
