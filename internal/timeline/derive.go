package timeline

import (
	"sort"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// Derive turns a parsed session into a timeline: one lane at a time, then merged in time order.
func Derive(s *session.Session, opts Options) *Timeline {
	opts = opts.withDefaults()
	tl := &Timeline{SessionID: s.ID}

	for _, lane := range s.Lanes {
		first, last, ok := laneSpan(lane)
		if !ok {
			continue // a lane whose records carry no timestamps has nothing to place in time
		}

		d := &laneDeriver{lane: lane, opts: opts, cursor: first, end: last, open: map[string]*call{}}
		rows := d.run()
		labelThinking(rows)

		tl.Lanes = append(tl.Lanes, LaneSpan{
			ID:         lane.ID,
			Name:       lane.Name,
			IsLead:     lane.IsLead,
			WorkflowID: lane.WorkflowID,
			Model:      lane.Meta.Model,
			Color:      lane.Meta.Color,
			First:      first,
			Last:       last,
			Rows:       len(rows),
		})
		tl.Rows = append(tl.Rows, rows...)

		if tl.First.IsZero() || first.Before(tl.First) {
			tl.First = first
		}
		if last.After(tl.Last) {
			tl.Last = last
		}
	}

	// nameWaits sweeps the lead's rows in time order, which they're in because a lane's cursor only moves forward and
	// the lead's lane goes in first. The sort after it is stable, so rows that start together keep their lane order.
	nameWaits(tl, opts)
	sort.SliceStable(tl.Rows, func(i, j int) bool { return tl.Rows[i].From.Before(tl.Rows[j].From) })
	return tl
}

// laneSpan is when a lane was alive: its first timestamped record, and the latest instant any of its records claims.
// The latest isn't always the last one in the file, because compaction replays records under older stamps.
func laneSpan(lane *session.Lane) (first, last time.Time, ok bool) {
	for _, rec := range lane.Records {
		if rec.Timestamp.IsZero() {
			continue
		}
		if !ok {
			first, last, ok = rec.Timestamp, rec.Timestamp, true
			continue
		}
		if rec.Timestamp.After(last) {
			last = rec.Timestamp
		}
	}
	return first, last, ok
}

// call is a tool call waiting for its result.
type call struct {
	block transcript.Block
	class ToolClass
	// subject is the short phrase naming what the call was about, reused by the call row and the execution row.
	subject string
	start   time.Time
	// end is when the result came back, zero while the call is still open.
	end time.Time
	// resolved says a result came back at all. A transcript captured mid-flight ends with calls that never resolve.
	resolved bool
	result   transcript.Block
	record   *transcript.Record
	line     int
}

// laneDeriver walks one lane's records and emits its rows.
//
// The cursor is the invariant: it only moves forward, and every row runs from where the cursor was to where it lands.
// That's what makes the rows tile the lane, and it's what absorbs a backwards timestamp into a zero-length row instead
// of a negative one.
type laneDeriver struct {
	lane *session.Lane
	opts Options

	cursor time.Time
	end    time.Time
	rows   []Row

	// state says whether the time passing is the lane working or the lane idle, and idleReason says what put it there,
	// which is all an idle row can say about itself.
	state      laneState
	idleReason string

	// open holds the calls of the current batch that haven't come back yet, and batch holds the whole batch in the
	// order it was issued. Both empty out together.
	open  map[string]*call
	batch []*call
}

func (d *laneDeriver) run() []Row {
	for _, rec := range d.lane.Records {
		if rec.Timestamp.IsZero() {
			continue // session state, not a moment in the lane's life
		}
		ts := d.forward(rec.Timestamp)

		switch {
		case rec.Type == transcript.TypeAssistant && rec.APIError != nil:
			// An assistant record, text block and all, but the harness wrote it rather than the model. It has to be
			// matched before the case below, which would otherwise call the error message the agent writing prose.
			d.flush(ts)
			d.emitAPIError(rec, ts)

		case rec.Type == transcript.TypeAssistant && len(rec.Blocks) > 0:
			d.flush(ts)
			if reason, idle := d.wasIdle(ts); idle {
				// Nothing on record says when the lane started working again, so the stretch counts as idle and the
				// block that closed it claims none of it. Nothing says what it was waiting for either.
				d.emitWait(ts, KindWaitingUnknown, reason, rec.Line)
			}
			d.state = laneWorking
			d.emitResponse(rec, ts)

		case rec.Type == transcript.TypeUser && hasToolResults(rec):
			d.state = laneWorking
			d.resolve(rec, ts)

		case rec.Type == transcript.TypeUser && isPrompt(rec):
			d.flush(ts)
			kind, info := waitedFor(rec)
			d.emitWait(ts, kind, info, rec.Line)
			d.state = laneWorking

		case rec.Type == transcript.TypeQueueOperation && isEnqueue(rec) && len(d.batch) == 0:
			// Input landing in the queue while no tool call is open. Its timestamp is when the input actually arrived,
			// which is where the wait ends and the next turn's thinking starts.
			//
			// This doesn't wait for a turn-end record, because the harness doesn't always write one: a lane in the
			// corpus sat silent for 7h23m after a text block, took a queued "Go on", and answered three seconds later,
			// and with the turn ending as the only signal all of it counted as thinking. Input arriving while the
			// agent is genuinely composing clips a few seconds off the front of that row instead, which is a bounded
			// error where the other one has no bound.
			kind, info := waitedFor(rec)
			d.emitWait(ts, kind, info, rec.Line)
			d.state = lanePending

		case rec.Type == transcript.TypeSystem && rec.System != nil && isTurnEnd(rec.System.Subtype):
			d.goIdle("idle after the turn ended")

		case rec.Type == transcript.TypeSystem && rec.System != nil && rec.System.Subtype == compactBoundary:
			d.flush(ts)
			d.emitCompaction(rec, ts)
		}
		// Anything else is bookkeeping: an attachment, a hook's output, a queue operation the lane wasn't waiting on.
		// It carries a timestamp but it isn't the lane doing something, so its time belongs to whichever row surrounds
		// it.
	}

	d.flush(d.end)
	d.closeTail()
	return d.rows
}

// forward clamps a record's timestamp to the cursor. Records stamped before the one they follow are ordinary: write
// jitter puts 186 of the reference session's 15,831 records there, and compaction replays a whole run of them.
func (d *laneDeriver) forward(ts time.Time) time.Time {
	if ts.Before(d.cursor) {
		return d.cursor
	}
	return ts
}

// emitResponse turns one assistant record into one row per content block.
//
// The blocks of a record share a single timestamp, so only the first of them can honestly claim the span that ended
// there. The rest are zero-length. Giving the span to the first block rather than splitting it is deliberate: on a
// modern transcript each block is its own record and the question doesn't arise, and on an older one the first block
// is the thinking block, which is where the time actually went.
func (d *laneDeriver) emitResponse(rec *transcript.Record, ts time.Time) {
	from := d.cursor
	for _, b := range rec.Blocks {
		row := Row{From: from, Until: ts, Agent: d.lane.Name, LaneID: d.lane.ID, Line: rec.Line}

		switch b.Type {
		case transcript.BlockThinking:
			row.Kind = KindThinking
			row.Info = thinkingInfo(b)
		case transcript.BlockText:
			row.Kind = KindWriting
			row.Info = clip(b.Text, subjectLimit)
		case transcript.BlockToolUse:
			c := &call{block: b, class: Classify(b), subject: subjectOf(b), start: ts, line: rec.Line}
			row.Kind = KindToolCall
			row.Tool = b.ToolName
			row.Class = c.class
			row.Info = callInfo(c)
			d.open[b.ToolUseID] = c
			d.batch = append(d.batch, c)
		default:
			// A block type nothing decodes gets no row and no time. `fallback`, which marks the harness switching
			// models mid-response, is the one seen in the wild, and it isn't the agent doing anything. Leaving the
			// cursor alone hands its stretch to the row that follows, which is where the time really went.
			continue
		}
		d.rows = append(d.rows, row)
		// The span belongs to the first block that produced a row, so everything after it is zero-length. A block
		// that produced none leaves the span where it was.
		from = ts
		d.cursor = ts
	}
}

// resolve matches a result record against the calls still open, and flushes the batch once they're all back.
func (d *laneDeriver) resolve(rec *transcript.Record, ts time.Time) {
	for _, b := range rec.Blocks {
		if b.Type != transcript.BlockToolResult {
			continue
		}
		c, ok := d.open[b.ToolUseID]
		if !ok {
			continue // a result for a call this lane never issued: nothing to time
		}
		delete(d.open, b.ToolUseID)
		c.end = ts
		if c.end.Before(c.start) {
			c.end = c.start
		}
		c.resolved = true
		c.result = b
		c.record = rec
		c.line = rec.Line
	}
	if len(d.open) == 0 {
		d.flush(ts)
	}
}

// flush turns a finished batch of calls into execution rows. Calls that came back are emitted in the order they came
// back, so the last one carries the lane forward and its siblings are the ones marked as overlapping. A call that
// never came back is closed at the moment that forced the flush, and says so.
func (d *laneDeriver) flush(ts time.Time) {
	if len(d.batch) == 0 {
		return
	}
	batch := d.batch
	d.batch = nil
	d.open = map[string]*call{}

	for _, c := range batch {
		if !c.resolved {
			c.end = ts
			if c.end.Before(c.start) {
				c.end = c.start
			}
		}
	}
	sort.SliceStable(batch, func(i, j int) bool { return batch[i].end.Before(batch[j].end) })

	parallel := len(batch) > 1
	for i, c := range batch {
		v := judge(c, d.opts)
		row := Row{
			From:       c.start,
			Until:      c.end,
			Agent:      d.lane.Name,
			LaneID:     d.lane.ID,
			Kind:       v.kind,
			Tool:       c.block.ToolName,
			Class:      c.class,
			Overlapped: parallel && i < len(batch)-1,
			TimedOut:   v.timedOut,
			IsError:    c.result.IsError,
			Line:       c.line,
		}
		row.Info = executionInfo(c, v, len(batch))
		d.rows = append(d.rows, row)
		if c.end.After(d.cursor) {
			d.cursor = c.end
		}
	}
}

// wasIdle says the stretch ending now was the lane sitting idle rather than the model working, and why.
//
// There are two ways to tell. The lane stopped and nothing has arrived since, which is evidence and carries its own
// reason (a turn ending, or a request the API refused). Or the stretch is longer than a model response can be, which is
// the backstop for lanes carrying no evidence at all: a session left after `/exit` and resumed 25 days later has
// nothing between the two but a text block.
func (d *laneDeriver) wasIdle(ts time.Time) (string, bool) {
	switch {
	case d.state == laneIdle:
		return d.idleReason, true
	case ts.Sub(d.cursor) > d.opts.MaxResponseSpan:
		return "idle, with nothing on record saying when the lane started working again", true
	default:
		return "", false
	}
}

// goIdle marks the lane as having stopped, and why. The reason is what an idle row that nothing else explains says
// about itself, so the two always move together.
func (d *laneDeriver) goIdle(reason string) {
	d.state, d.idleReason = laneIdle, reason
}

// emitWait closes an idle gap, in the kind that says what the lane was idle on. A zero-length wait is dropped: the
// lane's very first record is a prompt, and a wait of no time isn't worth a row.
func (d *laneDeriver) emitWait(ts time.Time, kind Kind, info string, line int) {
	if !ts.After(d.cursor) {
		return
	}
	d.rows = append(d.rows, Row{
		From:   d.cursor,
		Until:  ts,
		Agent:  d.lane.Name,
		LaneID: d.lane.ID,
		Kind:   kind,
		Info:   info,
		Line:   line,
	})
	d.cursor = ts
}

// closeTail covers the stretch after the lane's last row: bookkeeping records at the end of a transcript, or a lane
// that was still idle when the session was read.
func (d *laneDeriver) closeTail() {
	if !d.end.After(d.cursor) {
		return
	}
	d.rows = append(d.rows, Row{
		From:   d.cursor,
		Until:  d.end,
		Agent:  d.lane.Name,
		LaneID: d.lane.ID,
		Kind:   KindWaitingUnknown,
		Info:   "idle to the end of the transcript",
	})
	d.cursor = d.end
}

// hasToolResults says a user record is carrying results rather than something a person sent.
func hasToolResults(rec *transcript.Record) bool {
	for _, b := range rec.Blocks {
		if b.Type == transcript.BlockToolResult {
			return true
		}
	}
	return false
}

// isPrompt says a user record is something that arrived from outside the lane: a person typing, a teammate writing, or
// the harness handing the agent its task. Harness-injected records are marked and don't count.
func isPrompt(rec *transcript.Record) bool {
	if rec.IsMeta {
		return false
	}
	if rec.Prompt != "" {
		return true
	}
	for _, b := range rec.Blocks {
		if b.Type == transcript.BlockText || b.Type == transcript.BlockImage {
			return true
		}
	}
	return false
}

// laneState is what the lane was doing when the last record went by, which decides whether the stretch before the next
// one counts as the model working or as the lane sitting idle.
type laneState int

const (
	// laneWorking means the lane is mid-turn: the time passing is the model thinking or a tool running.
	laneWorking laneState = iota
	// laneIdle means the lane stopped and nothing has arrived since, so the time passing is idle. It stopped at a turn
	// ending or at a request the API refused, and idleReason says which.
	laneIdle
	// lanePending means input has arrived and the lane hasn't produced anything yet. It's where the next turn's
	// thinking starts from, so the stretch after it belongs to the model rather than to the wait.
	lanePending
)

// compactBoundary is the system record subtype that marks a finished compaction.
const compactBoundary = "compact_boundary"

// isTurnEnd says a system record marks the lane's turn ending. After one of these the lane is idle until something
// starts a new turn, which is what keeps an overnight gap from being reported as overnight thinking.
func isTurnEnd(subtype string) bool {
	switch subtype {
	case "turn_duration", "stop_hook_summary", "away_summary":
		return true
	}
	return false
}

// isEnqueue says a queue record is input arriving rather than the harness clearing the queue.
func isEnqueue(rec *transcript.Record) bool {
	return rec.Queue != nil && rec.Queue.Operation == "enqueue"
}

// emitCompaction turns a finished compaction into its own row, and leaves the time before it as a wait.
//
// The records around a compaction disagree about when it happened: the boundary is stamped when compaction finished,
// while the records replayed after it keep the stamps they had before, which is where the reference session's 132 s
// backwards jump comes from. So the row is measured back from the boundary using the duration the boundary itself
// reports, and the idle stretch that came before stays a wait rather than being swallowed.
func (d *laneDeriver) emitCompaction(rec *transcript.Record, ts time.Time) {
	start := ts
	if rec.System.Compact != nil && rec.System.Compact.Duration > 0 {
		start = ts.Add(-rec.System.Compact.Duration)
	}
	if start.Before(d.cursor) {
		start = d.cursor // a compaction can't reach back past work that already happened
	}

	if start.After(d.cursor) {
		d.rows = append(d.rows, Row{
			From:   d.cursor,
			Until:  start,
			Agent:  d.lane.Name,
			LaneID: d.lane.ID,
			Kind:   KindWaitingUnknown,
			Info:   "idle until the context was compacted",
			Line:   rec.Line,
		})
	}
	d.rows = append(d.rows, Row{
		From:   start,
		Until:  ts,
		Agent:  d.lane.Name,
		LaneID: d.lane.ID,
		Kind:   KindCompacting,
		Info:   compactionInfo(rec.System.Compact),
		Line:   rec.Line,
	})
	d.cursor = ts
}
