package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

// defaultLimit keeps the listing to a screen. There are 722 sessions on the machine this was built against, and the
// one a person is after is nearly always a recent one.
const defaultLimit = 20

// titleWidth clips a title so the column stays a column. Titles run long, and the id is what gets copied anyway.
const titleWidth = 44

func runSessions(a *app, args []string) error {
	fs := newFlagSet(a, "sessions")
	root := fs.String("root", "", "read transcripts from this directory instead of ~/.claude/projects")
	limit := fs.Int("limit", defaultLimit, "show at most this many sessions, or 0 for all of them")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("`%s sessions` doesn't take arguments, and it got %q.", binary, rest[0])
	}

	dir, err := transcriptRoot(*root)
	if err != nil {
		return err
	}
	sums, err := session.List(dir)
	if err != nil {
		if problem := missingRoot(dir, err); problem != nil {
			return problem
		}
		return fmt.Errorf("Couldn't read the transcripts in %s: %w", dir, err)
	}
	if len(sums) == 0 {
		fmt.Fprintf(a.out, "No sessions under %s yet.\n", dir)
		return nil
	}

	shown := sums
	if *limit > 0 && *limit < len(shown) {
		shown = shown[:*limit]
	}

	tw := tabwriter.NewWriter(a.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Session\tStarted\tLength\tLanes\tSize\tProject\tTitle")
	for _, s := range shown {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			s.ID, localTime(s.Start), length(s), count(s.Lanes), humanBytes(s.Bytes),
			shortenHome(s.ProjectPath), clip(s.Title, titleWidth))
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("Couldn't write the listing: %w", err)
	}

	var lanes int
	var bytes int64
	for _, s := range sums {
		lanes += s.Lanes
		bytes += s.Bytes
	}
	if len(shown) < len(sums) {
		fmt.Fprintf(a.out, "\nShowing %s of %s sessions (%s lanes, %s on disk). Use `--limit 0` for all of them.\n",
			count(len(shown)), count(len(sums)), count(lanes), humanBytes(bytes))
		return nil
	}
	fmt.Fprintf(a.out, "\n%s sessions, %s lanes, %s on disk. Times are local.\n",
		count(len(sums)), count(lanes), humanBytes(bytes))
	return nil
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
