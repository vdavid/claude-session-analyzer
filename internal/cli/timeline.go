package cli

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"

	"github.com/vdavid/claude-session-analyzer/internal/report"
	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

func runTimeline(a *app, args []string) error {
	fs := newFlagSet(a, "timeline")
	root := fs.String("root", "", "read transcripts from this directory instead of ~/.claude/projects")
	out := fs.String("out", "", "write the output to this file instead of standard output")
	asJSON := fs.Bool("json", false, "write the aggregates as JSON instead of a CSV of rows")
	withRows := fs.Bool("rows", false, "include every row in the JSON, which runs to megabytes on a large session")
	ids, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	switch {
	case len(ids) == 1 && ids[0] != "":
	case len(ids) < 2:
		return usagef("Which session? Pass a session id or a unique prefix of one. `%s sessions` lists what's on disk.",
			binary)
	default:
		return usagef("One session at a time, and this got %d. `%s timeline <session-id>`.", len(ids), binary)
	}

	dir, err := transcriptRoot(*root)
	if err != nil {
		return err
	}
	loc, err := resolve(dir, ids[0])
	if err != nil {
		return err
	}

	sum, err := session.Summarize(loc)
	if err != nil {
		return fmt.Errorf("Couldn't read session %s: %w", loc.ID, err)
	}
	s, err := session.Load(loc, transcript.Options{})
	if err != nil {
		return fmt.Errorf("Couldn't read session %s: %w", loc.ID, err)
	}
	tl := timeline.Derive(s, timeline.Options{})

	// The JSON leaves the rows out unless they're asked for. A caller wanting every row wants the CSV, and a 983-lane
	// session's rows are 8 MB, which is a bad thing to hand an agent by accident.
	write := func(w io.Writer) error { return writeCSV(w, tl) }
	if *asJSON {
		write = func(w io.Writer) error { return writeJSON(w, report.ForTimeline(sum, tl, *withRows)) }
	}

	if *out == "" {
		return write(a.out)
	}

	f, err := os.Create(*out)
	if err != nil {
		return fmt.Errorf("Couldn't create %s: %w", *out, err)
	}
	defer f.Close() //nolint:errcheck // closed again below, where the error matters

	if err := write(f); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("Couldn't finish writing %s: %w", *out, err)
	}

	// The note goes to stderr so that redirecting stdout to a file and passing `--out` both give the CSV alone.
	fmt.Fprintf(a.err, "Wrote %s rows across %s lanes to %s, covering %s.\n",
		count(len(tl.Rows)), count(len(tl.Lanes)), *out, timeline.FormatDuration(tl.Duration()))
	return nil
}

func writeCSV(w io.Writer, tl *timeline.Timeline) error {
	cw := csv.NewWriter(w)
	if err := cw.WriteAll(tl.Records()); err != nil {
		return fmt.Errorf("Couldn't write the CSV: %w", err)
	}
	return nil
}
