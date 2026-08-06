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

	nameWaits(tl)
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
		case rec.Type == transcript.TypeAssistant && len(rec.Blocks) > 0:
			d.flush(ts)
			d.emitResponse(rec, ts)

		case rec.Type == transcript.TypeUser && hasToolResults(rec):
			d.resolve(rec, ts)

		case rec.Type == transcript.TypeUser && isPrompt(rec):
			d.flush(ts)
			d.emitWait(rec, ts)

		case rec.Type == transcript.TypeSystem && rec.System != nil && rec.System.Subtype == compactBoundary:
			d.flush(ts)
			d.emitCompaction(rec, ts)
		}
		// Anything else is bookkeeping: an attachment, a turn summary, a queue operation. It carries a timestamp but
		// it isn't the lane doing something, so its time belongs to whichever row surrounds it.
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
		from = ts // every block after the first is zero-length

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
			continue // an unexpected block type costs no time rather than costing us the record
		}
		d.rows = append(d.rows, row)
	}
	d.cursor = ts
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

// emitWait closes the gap before a prompt landed. A zero-length wait is dropped: the lane's very first record is a
// prompt, and a wait of no time isn't worth a row.
func (d *laneDeriver) emitWait(rec *transcript.Record, ts time.Time) {
	if !ts.After(d.cursor) {
		d.cursor = ts
		return
	}
	d.rows = append(d.rows, Row{
		From:   d.cursor,
		Until:  ts,
		Agent:  d.lane.Name,
		LaneID: d.lane.ID,
		Kind:   KindWaiting,
		Info:   waitInfo(rec),
		Line:   rec.Line,
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
		Kind:   KindWaiting,
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

// compactBoundary is the system record subtype that marks a finished compaction.
const compactBoundary = "compact_boundary"

// emitCompaction turns a finished compaction into its own row.
func (d *laneDeriver) emitCompaction(rec *transcript.Record, ts time.Time) {}
