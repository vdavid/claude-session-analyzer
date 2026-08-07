package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/cache"
	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/stats"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

// defaultTop bounds what a query prints. An answer an agent reads goes into a context window, so the default is a
// screenful and `--top 0` is the way to ask for all of it.
const defaultTop = 20

func runStats(a *app, args []string) error {
	fs := newFlagSet(a, "stats")
	root := fs.String("root", "", "read transcripts from this directory instead of ~/.claude/projects")
	groupBy := fs.String("group-by", "", "group the answer by these dimensions, comma separated")
	top := fs.Int("top", defaultTop, "keep only this many groups, or 0 for all of them")
	asJSON := fs.Bool("json", false, "write the answer as JSON")
	noCache := fs.Bool("no-cache", false, "parse every session instead of reading the digest cache")
	vocabulary := fs.Bool("vocabulary", false, "list the dimensions, activity kinds, and tool classes a query can name")
	var where stringList
	fs.Var(&where, "where", "keep only cells matching `field=value`, repeatable, comma separated for several values")
	var only scope
	only.register(fs)

	ids, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if *vocabulary {
		return writeVocabulary(a, *asJSON)
	}
	if len(ids) > 1 {
		return usagef("One session at a time, and this got %d. Leave the id out to ask about all of them.", len(ids))
	}

	// A query that doesn't parse is a mistake in the command line, so it exits 2 like every other one. The engine's
	// message already names what was wrong and what's valid.
	spec, err := stats.ParseSpec(where, *groupBy, *top)
	if err != nil {
		return usagef("%s", err)
	}
	keep, err := only.filter()
	if err != nil {
		return err
	}

	dir, err := transcriptRoot(*root)
	if err != nil {
		return err
	}

	// A session id narrows to one session, and it's resolved first so an unknown or ambiguous prefix is answered the
	// way every other command answers it rather than as "no sessions matched".
	if len(ids) == 1 && ids[0] != "" {
		loc, err := resolve(dir, ids[0])
		if err != nil {
			return err
		}
		if only.narrowed() {
			return usagef("`--since`, `--until`, and `--project` narrow the corpus, and this already named one session.")
		}
		keep = func(sum session.Summary) bool { return sum.ID == loc.ID }
	}

	store, err := cache.Open(dir)
	if err != nil {
		return err
	}
	walk, err := store.Corpus(dir, cache.CorpusOptions{
		Include:  keep,
		Zone:     time.Local,
		NoCache:  *noCache,
		Detail:   spec.NeedsLanes(),
		Progress: progressTo(a, len(ids) == 0),
	})
	if err != nil {
		if problem := missingRoot(dir, err); problem != nil {
			return problem
		}
		return fmt.Errorf("Couldn't read the transcripts in %s: %w", dir, err)
	}
	if len(walk.Digests) == 0 {
		return fmt.Errorf("No sessions under %s match that. `%s sessions` lists what's on disk.", dir, binary)
	}

	result, err := stats.Run(spec, sources(walk, spec.NeedsLanes()))
	if err != nil {
		return err
	}
	if walk.Failed > 0 {
		result.Notes = append(result.Notes,
			fmt.Sprintf("%s sessions couldn't be read and are left out of every number here.", count(walk.Failed)))
	}

	if *asJSON {
		return writeJSON(a.out, statsBody{Result: result, Source: statsSource{
			Root: dir, Cached: walk.Hits, Parsed: walk.Parsed, Failed: walk.Failed,
		}})
	}
	return writeStatsTable(a, result, walk, spec)
}

// statsBody is the JSON answer: the query's own result, plus where the numbers came from. A reader checking a
// surprising number wants to know how much of it was served from a cache.
type statsBody struct {
	stats.Result
	Source statsSource `json:"source"`
}

type statsSource struct {
	Root   string `json:"root"`
	Cached int    `json:"cached"`
	Parsed int    `json:"parsed"`
	Failed int    `json:"failed,omitempty"`
}

// sources turns a walk into what the engine queries. Tier two replaces tier one whenever it was loaded, because the
// two hold the same totals and only tier two can answer for a lane.
func sources(walk cache.CorpusResult, withLanes bool) []stats.Source {
	out := make([]stats.Source, 0, len(walk.Digests))
	for _, d := range walk.Digests {
		src := stats.Source{
			SessionID:        d.Session.ID,
			ProjectName:      d.Session.ProjectName,
			ProjectSlug:      d.Session.ProjectSlug,
			Title:            d.Session.Title,
			Cells:            d.Cube(),
			WallClockSeconds: d.Totals.WallClockSeconds,
			Lanes:            d.Totals.Lanes,
		}
		if withLanes {
			if detail, ok := walk.Details[d.Session.ID]; ok {
				src.Cells = detail.Cube()
			}
		}
		out = append(out, src)
	}
	return out
}

// progressTo says something during the half minute a cold corpus walk takes. It goes to stderr, so `--json` piped to a
// file still gets the answer alone, and it's off for a single session, which is fast enough to need no commentary.
func progressTo(a *app, corpus bool) func(done, total int) {
	if !corpus {
		return nil
	}
	var last time.Time
	return func(done, total int) {
		if done < total && time.Since(last) < time.Second {
			return
		}
		last = time.Now()
		fmt.Fprintf(a.err, "\rReading %s of %s sessions...", count(done), count(total))
		if done == total {
			fmt.Fprintln(a.err)
		}
	}
}

func writeStatsTable(a *app, result stats.Result, walk cache.CorpusResult, spec stats.Spec) error {
	fmt.Fprintf(a.out, "%s across %s (%s cached, %s parsed).\n",
		coveringDays(result.Scope), plural(result.Totals.Sessions, "session"), count(walk.Hits), count(walk.Parsed))

	if len(result.Groups) > 0 {
		// Calls and sessions answer different questions, how often and how widely, and both are narrow enough to keep.
		// The session count only earns its column where it can differ between rows: grouping by session puts a 1 on
		// every one, and a single session in scope does the same.
		sessions := countsSessions(result, spec)

		tw := tabwriter.NewWriter(a.out, 0, 0, 2, ' ', 0)
		headers := make([]string, 0, len(spec.GroupBy)+4)
		for _, dim := range spec.GroupBy {
			headers = append(headers, strings.ToUpper(string(dim)[:1])+string(dim)[1:])
		}
		headers = append(headers, "Time", "Calls")
		if sessions {
			headers = append(headers, "Sessions")
		}
		// The share is against lane time, because that's the denominator the groups partition: an unfiltered query's
		// column adds to 100%. Against active time a waiting group would read as 94% of a total it isn't part of.
		fmt.Fprintln(tw, strings.Join(append(headers, "Share of lane time"), "\t"))
		for _, g := range result.Groups {
			cells := make([]string, 0, len(headers)+1)
			for _, dim := range spec.GroupBy {
				cells = append(cells, clip(g.Key.Value(dim), titleWidth))
			}
			cells = append(cells,
				timeline.FormatDuration(asDuration(g.Seconds)),
				count(g.Calls))
			if sessions {
				cells = append(cells, count(g.Sessions))
			}
			cells = append(cells, percent(g.ShareOfLaneTime))
			fmt.Fprintln(tw, strings.Join(cells, "\t"))
		}
		if err := tw.Flush(); err != nil {
			return fmt.Errorf("Couldn't write the answer: %w", err)
		}
		if result.Truncated > 0 {
			fmt.Fprintf(a.out, "\n%s more groups. Use `--top 0` for all of them.\n", count(result.Truncated))
		}
	}

	// Three denominators, named. The JSON carries a share against each; the table leads with lane time and spells the
	// other two out, because "active" excluding the waiting is exactly the thing a reader has to know before dividing.
	m := result.Matched
	in := ""
	if result.Totals.Sessions > 1 {
		in = fmt.Sprintf(" in %s of %s", count(m.Sessions), plural(result.Totals.Sessions, "session"))
	}
	fmt.Fprintf(a.out, "\nMatched %s over %s%s, which is %s of lane time.\n",
		timeline.FormatDuration(asDuration(m.Seconds)), plural(m.Calls, "call"), in, percent(m.ShareOfLaneTime))
	fmt.Fprintf(a.out, "Out of %s lane time (every agent's clock added up), %s active (waiting, stalls, and API "+
		"errors taken out), %s wall clock.\n",
		timeline.FormatDuration(asDuration(result.Totals.LaneTimeSeconds)),
		timeline.FormatDuration(asDuration(result.Totals.ActiveSeconds)),
		timeline.FormatDuration(asDuration(result.Totals.WallClockSeconds)))

	for _, note := range result.Notes {
		fmt.Fprintf(a.out, "\n%s\n", note)
	}
	return nil
}

// countsSessions says a per-group session count would tell a reader something it doesn't already know. Grouping by
// session puts a 1 on every row, and a query narrowed to one session can't do better than that either.
func countsSessions(result stats.Result, spec stats.Spec) bool {
	if result.Totals.Sessions <= 1 {
		return false
	}
	for _, dim := range spec.GroupBy {
		if dim == stats.DimSession {
			return false
		}
	}
	return true
}

func writeVocabulary(a *app, asJSON bool) error {
	vocab := stats.Vocabulary()
	if asJSON {
		return writeJSON(a.out, vocab)
	}
	fmt.Fprintf(a.out, "Dimensions, for `--group-by` and as `--where` fields:\n  %s\n\n", join(vocab.Dims))
	fmt.Fprintf(a.out, "Activity kinds (`--where kind=...`):\n  %s\n\n", strings.Join(vocab.Kinds, ", "))
	fmt.Fprintf(a.out, "Tool classes (`--where class=...`):\n  %s\n\n", strings.Join(vocab.Classes, ", "))
	fmt.Fprintf(a.out, "Groups, leaves, and tools are whatever a session used. "+
		"`%s stats <id> --group-by group` lists the ones it reached for.\n", binary)
	return nil
}

func join(dims []stats.Dim) string {
	out := make([]string, 0, len(dims))
	for _, d := range dims {
		out = append(out, string(d))
	}
	return strings.Join(out, ", ")
}

func coveringDays(s stats.Scope) string {
	switch {
	case s.FirstDay == "":
		return "Nothing timestamped"
	case s.FirstDay == s.LastDay:
		return s.FirstDay
	default:
		return s.FirstDay + " to " + s.LastDay
	}
}

func share(part, whole float64) float64 {
	if whole == 0 {
		return 0
	}
	return part / whole
}

func percent(fraction float64) string { return fmt.Sprintf("%.1f%%", fraction*100) }

func asDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

func plural(n int, noun string) string {
	if n == 1 {
		return count(n) + " " + noun
	}
	return count(n) + " " + noun + "s"
}

// stringList collects a flag given more than once, which is how `--where` ANDs two conditions.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, " ") }

func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}
