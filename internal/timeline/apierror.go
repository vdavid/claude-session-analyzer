package timeline

import (
	"strconv"
	"strings"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// emitAPIError turns a request the API didn't answer into its own row.
//
// The span is the stretch before the record, which is the harness retrying: time the session lost through no fault of
// the agent. Nothing else in the transcript measures an outage, because no retry is written down and no transcript in
// the corpus holds two error records in a row.
//
// Two clamps keep the span honest, and both leave a wait behind rather than swallowing time they can't account for:
//
//   - A lane whose turn had already ended had no request in flight, so the stretch before the error was the lane
//     sitting idle. The error marks the moment and claims none of it.
//   - The retry window can't run longer than MaxAPIErrorSpan, so a session resumed weeks later onto an expired login
//     reports the weeks as idle and the outage as the cap.
func (d *laneDeriver) emitAPIError(rec *transcript.Record, ts time.Time) {
	start := ts
	if d.state != laneIdle {
		start = ts.Add(-d.opts.MaxAPIErrorSpan)
	}
	if start.Before(d.cursor) {
		start = d.cursor
	}
	if start.After(d.cursor) {
		reason := d.idleReason
		if d.state != laneIdle {
			reason = "idle, with nothing on record saying when the failed request was made"
		}
		d.emitWait(start, KindWaitingUnknown, reason, rec.Line)
	}

	d.rows = append(d.rows, Row{
		From:   d.cursor,
		Until:  ts,
		Agent:  d.lane.Name,
		LaneID: d.lane.ID,
		Kind:   KindAPIError,
		Info:   apiErrorInfo(rec),
		Line:   rec.Line,
	})
	d.cursor = ts

	// The API refused, so the agent can't carry on by itself: whatever comes next was started by something else.
	d.goIdle("idle after the API call failed")
}

// apiErrorInfo labels an API-error row: the typed reason, the status, and what the harness showed the person.
//
// The typed fields are what it's read from. The prose is quoted rather than parsed, because it's copy: it varies by
// version and by locale, and the same failure has been worded four ways across the corpus.
func apiErrorInfo(rec *transcript.Record) string {
	e := rec.APIError
	if e == nil {
		return "the API didn't answer"
	}

	// Underscores out rather than a table of phrasings: a value nobody has seen yet still reads as English.
	info := strings.ReplaceAll(e.Kind, "_", " ")
	if info == "" {
		info = "the API didn't answer"
	}
	if e.Status != 0 {
		info += " (" + strconv.Itoa(e.Status) + ")"
	}
	for _, b := range rec.Blocks {
		if b.Type == transcript.BlockText && b.Text != "" {
			return info + ": " + clip(b.Text, subjectLimit)
		}
	}
	return info
}
