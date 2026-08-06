package timeline

import (
	"strconv"
	"time"
)

// timeLayout is how a row's instants are written: RFC 3339 in UTC, to the millisecond. Transcripts are stamped in UTC,
// and sub-second rows are worth keeping, so the column keeps both.
const timeLayout = "2006-01-02T15:04:05.000Z07:00"

// Columns are the CSV header, in order. The first five are the ones David asked for; the duration is last because
// every consumer wants it and it costs nothing.
func Columns() []string {
	return []string{"From", "Until", "Agent", "Activity", "Extra info", "Duration (s)"}
}

// Fields renders one row in the order Columns lists.
func (r Row) Fields() []string {
	return []string{
		r.From.UTC().Format(timeLayout),
		r.Until.UTC().Format(timeLayout),
		r.Agent,
		string(r.Kind),
		r.Info,
		strconv.FormatFloat(r.Duration().Seconds(), 'f', 3, 64),
	}
}

// Records renders the whole timeline for a CSV writer, header first.
func (t *Timeline) Records() [][]string {
	out := make([][]string, 0, len(t.Rows)+1)
	out = append(out, Columns())
	for _, r := range t.Rows {
		out = append(out, r.Fields())
	}
	return out
}

// TotalsByKind adds up how long the session spent on each activity kind, in the order Kinds lists. Parallel tool
// executions overlap, so the total can exceed the session's wall clock, which is the honest answer rather than a
// rounded one.
func (t *Timeline) TotalsByKind() map[Kind]time.Duration {
	totals := make(map[Kind]time.Duration, len(Kinds))
	for _, r := range t.Rows {
		totals[r.Kind] += r.Duration()
	}
	return totals
}
