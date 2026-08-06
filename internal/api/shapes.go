package api

import (
	"math"
	"path/filepath"
	"sort"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

// The JSON these shapes render is the contract the web app is built against. `docs/api.md` describes it, and changing
// a field here means changing that.
//
// Every duration is seconds, as a number, named `seconds` or `...Seconds`. Every instant is RFC 3339 in UTC. A
// timestamp that isn't known is null rather than a zero date, because 99 of the 725 sessions on this machine carry no
// timestamped record at all and 1 January year 1 would be a lie.

type sessionListBody struct {
	// Root is where the sessions were read from, so the UI can say so without guessing.
	Root     string        `json:"root"`
	Sessions []sessionBody `json:"sessions"`
	// Totals cover every session under the root, not only the ones a limit let through.
	Totals sessionListTotals `json:"totals"`
}

type sessionListTotals struct {
	Sessions  int   `json:"sessions"`
	Subagents int   `json:"subagents"`
	Bytes     int64 `json:"bytes"`
}

type oneSessionBody struct {
	Session sessionBody `json:"session"`
}

// sessionBody is one session as the list and the session header show it. It costs two reads of the lead transcript's
// ends, so it stays cheap on a root holding thousands of sessions.
type sessionBody struct {
	ID          string `json:"id"`
	ProjectSlug string `json:"projectSlug"`
	ProjectPath string `json:"projectPath"`
	// ProjectName is the path's last element, which is what a list wants to show.
	ProjectName string `json:"projectName"`
	Title       string `json:"title"`
	// Start and End are the lead transcript's first and last timestamped records. A timeline's own span covers every
	// lane, so it's reported separately, in totals.
	Start   *time.Time `json:"start"`
	End     *time.Time `json:"end"`
	Seconds float64    `json:"seconds"`
	// Modified is the lead transcript's mtime, which is all a session with no timestamped records has.
	Modified time.Time `json:"modified"`
	// Subagents counts the lanes the session spawned. The lead isn't one of them, so this is one less than a
	// timeline's `totals.lanes`, and a session that spawned none reports zero.
	Subagents int `json:"subagents"`
	// Bytes is the lead transcript plus every subagent's.
	Bytes int64 `json:"bytes"`
}

type timelineBody struct {
	Session sessionBody `json:"session"`
	Totals  totalsBody  `json:"totals"`
	Lanes   []laneBody  `json:"lanes"`
	// Rows is every activity row, sorted by start. It's empty when the request asked for `rows=false`, and Totals.Rows
	// still says how many there were.
	Rows []rowBody `json:"rows"`
}

// totalsBody is the aggregation the browser would otherwise do over 15,000 rows.
type totalsBody struct {
	// From and Until bracket every lane, the subagents included, so they can run slightly wider than the session's own
	// start and end. Both are null on a session whose records carry no timestamp, same as a session's own Start and
	// End: a zero date renders as year one, which is a lie a reader has no way to spot.
	From  *time.Time `json:"from"`
	Until *time.Time `json:"until"`
	// WallClockSeconds is From to Until: how long the session took. LaneTimeSeconds is every lane's rows added up,
	// which is larger whenever lanes ran at the same time. They answer different questions and are never the same
	// number in a multi-agent session; don't present one as the other.
	WallClockSeconds float64 `json:"wallClockSeconds"`
	LaneTimeSeconds  float64 `json:"laneTimeSeconds"`
	Rows             int     `json:"rows"`
	Lanes            int     `json:"lanes"`
	// ByKind is the pie: one entry per activity kind that took time, in legend order.
	ByKind []kindTotal `json:"byKind"`
	// ByTool is the tool breakdown, biggest first: one entry per group, each holding the exact tools inside it.
	ByTool []toolGroupTotal `json:"byTool"`
}

type kindTotal struct {
	Kind    string  `json:"kind"`
	Seconds float64 `json:"seconds"`
	Rows    int     `json:"rows"`
}

// toolGroupTotal is one slice of the tool breakdown: everything a session did with one tool, at the level a reader
// asks about. `Bash` is a dozen tools wearing one name and an MCP server arrives as one tool per method, so the group
// is what the engine's ToolID calls it: `Bash (git)`, `codegraph (MCP)`, `Read`.
//
// The counts are calls, not rows. Every call has one row for the agent composing it and one for the tool running, and
// only the second is counted, so Calls is the number of times the tool was actually used and Seconds is what it cost.
type toolGroupTotal struct {
	Group string `json:"group"`
	// Class is what kind of work the group does, which is what colours it. Every tool in a group shares it.
	Class   string  `json:"class"`
	Calls   int     `json:"calls"`
	Seconds float64 `json:"seconds"`
	// Lanes is how many lanes made one of these calls, which answers "who used this" without reading the rows. It's
	// counted per group rather than added up from the tools, because one lane calling two of them is still one lane.
	Lanes    int `json:"lanes"`
	Errors   int `json:"errors,omitempty"`
	TimedOut int `json:"timedOut,omitempty"`
	// Tools are the exact things that ran inside the group, biggest first.
	Tools []toolTotal `json:"tools"`
}

// toolTotal is one exact tool inside a group.
type toolTotal struct {
	// Tool is the raw name the harness used, so a reader can grep a transcript for it. Leaf is the part that varies
	// inside the group: an MCP method, or the program a Bash call ran.
	Tool     string  `json:"tool"`
	Leaf     string  `json:"leaf"`
	Calls    int     `json:"calls"`
	Seconds  float64 `json:"seconds"`
	Lanes    int     `json:"lanes"`
	Errors   int     `json:"errors,omitempty"`
	TimedOut int     `json:"timedOut,omitempty"`
}

// laneBody is one swimlane: when the lane was alive, where its holes are, and what it spent its time on.
type laneBody struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	IsLead bool   `json:"isLead"`
	// WorkflowID names the workflow that spawned the lane, absent for one the session spawned directly.
	WorkflowID string `json:"workflowId,omitempty"`
	Model      string `json:"model,omitempty"`
	// Color is what the terminal used, and it's often missing, so the UI needs a palette of its own.
	Color   string      `json:"color,omitempty"`
	From    time.Time   `json:"from"`
	Until   time.Time   `json:"until"`
	Seconds float64     `json:"seconds"`
	Rows    int         `json:"rows"`
	ByKind  []kindTotal `json:"byKind"`
	// Gaps are the stretches the lane produced nothing: waiting, and stalled. Drawing a lane as one solid bar between
	// From and Until would claim it was busy the whole time.
	Gaps []gapBody `json:"gaps"`
}

type gapBody struct {
	From    time.Time `json:"from"`
	Until   time.Time `json:"until"`
	Seconds float64   `json:"seconds"`
	Kind    string    `json:"kind"`
	Info    string    `json:"info,omitempty"`
}

// rowBody is one activity row. It carries everything the derivation knows, which is more than the CSV's six columns.
type rowBody struct {
	From    time.Time `json:"from"`
	Until   time.Time `json:"until"`
	Seconds float64   `json:"seconds"`
	// LaneID is what to group by. Two lanes share a name when neither has a `.meta.json`.
	LaneID string `json:"laneId"`
	Agent  string `json:"agent"`
	Kind   string `json:"kind"`
	Info   string `json:"info,omitempty"`
	Tool   string `json:"tool,omitempty"`
	Class  string `json:"class,omitempty"`
	// ToolGroup is which slice of the tool breakdown the row belongs to, so a click on one can filter to its rows
	// without the browser re-deriving a rule that lives in the engine. The leaf is left out: nothing filters by it,
	// and the breakdown already carries the leaves it needs.
	ToolGroup string `json:"toolGroup,omitempty"`
	// Overlapped marks a row that ran alongside a sibling in the same lane, which is the only thing that breaks the
	// lane's rows tiling its span.
	Overlapped bool `json:"overlapped,omitempty"`
	TimedOut   bool `json:"timedOut,omitempty"`
	IsError    bool `json:"isError,omitempty"`
	// Line is the 1-based line of the lane's transcript that closed the row, for tracing a row back to its source. A
	// lane written across several files has one line numbering per file, so this isn't unique within a lane.
	Line int `json:"line,omitempty"`
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	// Code is what a caller branches on. Message is what a person reads.
	Code    string `json:"code"`
	Message string `json:"message"`
	// Matches names the candidates when an id matched more than one session.
	Matches []string `json:"matches,omitempty"`
}

func toSession(s session.Summary) sessionBody {
	body := sessionBody{
		ID:          s.ID,
		ProjectSlug: s.ProjectSlug,
		ProjectPath: s.ProjectPath,
		Title:       s.Title,
		Modified:    s.Modified.UTC(),
		Subagents:   s.Subagents,
		Bytes:       s.Bytes,
	}
	if s.ProjectPath != "" {
		body.ProjectName = filepath.Base(s.ProjectPath)
	}
	if !s.Start.IsZero() {
		start := s.Start.UTC()
		body.Start = &start
	}
	if !s.End.IsZero() {
		end := s.End.UTC()
		body.End = &end
	}
	if body.Start != nil && body.End != nil {
		body.Seconds = seconds(s.Duration())
	}
	return body
}

// buildTimeline aggregates a derived timeline into what the frontend draws. The browser gets the pie and the swimlane
// already summed, because summing 15,000 rows in JavaScript is the wrong half of the split when the backend is Go.
func buildTimeline(sum session.Summary, tl *timeline.Timeline, withRows bool) timelineBody {
	body := timelineBody{
		Session: toSession(sum),
		Totals: totalsBody{
			From:             nilable(tl.First),
			Until:            nilable(tl.Last),
			WallClockSeconds: seconds(tl.Duration()),
			Rows:             len(tl.Rows),
			Lanes:            len(tl.Lanes),
		},
		Lanes: make([]laneBody, 0, len(tl.Lanes)),
		Rows:  []rowBody{},
	}

	// One pass over the rows fills every aggregate. Durations add up as nanoseconds and become seconds once, so the
	// totals and the rows can't drift apart at the third decimal.
	type laneAgg struct {
		byKind map[timeline.Kind]*tally
		gaps   []gapBody
		rows   int
	}
	aggs := make(map[string]*laneAgg, len(tl.Lanes))
	for _, lane := range tl.Lanes {
		aggs[lane.ID] = &laneAgg{byKind: map[timeline.Kind]*tally{}}
	}

	overall := map[timeline.Kind]*tally{}
	tools := toolTallies{}
	var laneTime time.Duration

	if withRows {
		body.Rows = make([]rowBody, 0, len(tl.Rows))
	}
	for _, r := range tl.Rows {
		laneTime += r.Duration()
		add(overall, r)
		tools.add(r)

		if agg, ok := aggs[r.LaneID]; ok {
			agg.rows++
			add(agg.byKind, r)
			if r.Kind.IsGap() {
				agg.gaps = append(agg.gaps, gapBody{
					From:    r.From.UTC(),
					Until:   r.Until.UTC(),
					Seconds: seconds(r.Duration()),
					Kind:    string(r.Kind),
					Info:    r.Info,
				})
			}
		}
		if withRows {
			body.Rows = append(body.Rows, toRow(r))
		}
	}

	body.Totals.LaneTimeSeconds = seconds(laneTime)
	body.Totals.ByKind = inLegendOrder(overall)
	body.Totals.ByTool = tools.render()

	for _, lane := range tl.Lanes {
		agg := aggs[lane.ID]
		body.Lanes = append(body.Lanes, laneBody{
			ID:         lane.ID,
			Name:       lane.Name,
			IsLead:     lane.IsLead,
			WorkflowID: lane.WorkflowID,
			Model:      lane.Model,
			Color:      lane.Color,
			From:       lane.First.UTC(),
			Until:      lane.Last.UTC(),
			Seconds:    seconds(lane.Duration()),
			Rows:       agg.rows,
			ByKind:     inLegendOrder(agg.byKind),
			Gaps:       gapsOrNone(agg.gaps),
		})
	}
	return body
}

func toRow(r timeline.Row) rowBody {
	return rowBody{
		From:       r.From.UTC(),
		Until:      r.Until.UTC(),
		Seconds:    seconds(r.Duration()),
		LaneID:     r.LaneID,
		Agent:      r.Agent,
		Kind:       string(r.Kind),
		Info:       r.Info,
		Tool:       r.Tool,
		Class:      string(r.Class),
		ToolGroup:  r.ToolGroup,
		Overlapped: r.Overlapped,
		TimedOut:   r.TimedOut,
		IsError:    r.IsError,
		Line:       r.Line,
	}
}

// tally is a per-kind running total. It holds a duration rather than seconds, so a hundred thousand rows can't drift
// the totals away from the rows they were added from.
type tally struct {
	rows  int
	total time.Duration
}

func add(into map[timeline.Kind]*tally, r timeline.Row) {
	t, ok := into[r.Kind]
	if !ok {
		t = &tally{}
		into[r.Kind] = t
	}
	t.rows++
	t.total += r.Duration()
}

// toolTallies counts a session's tool use, by group and by the exact tool inside it. Keyed by name rather than held in
// a slice, because a session reaches for the same tool thousands of times and the order only matters once, at the end.
type toolTallies map[string]*groupTally

type groupTally struct {
	class string
	counts
	// lanes is the set of lanes that made one of these calls. A group's lane count can't be added up from its tools,
	// because one lane calling two of them is still one lane, so both levels keep their own set.
	lanes map[string]bool
	tools map[string]*leafTally
}

type leafTally struct {
	tool string
	counts
	lanes map[string]bool
}

// counts is what both levels of the breakdown hold.
type counts struct {
	calls    int
	total    time.Duration
	errors   int
	timedOut int
}

func (c *counts) add(r timeline.Row) {
	c.calls++
	c.total += r.Duration()
	if r.IsError {
		c.errors++
	}
	if r.TimedOut {
		c.timedOut++
	}
}

// add counts one row, if it's the row a tool call actually ran in. The composing row beside it isn't the tool doing
// anything, and counting both would report every call twice.
func (t toolTallies) add(r timeline.Row) {
	if !r.IsToolRun() {
		return
	}
	group, ok := t[r.ToolGroup]
	if !ok {
		group = &groupTally{class: string(r.Class), lanes: map[string]bool{}, tools: map[string]*leafTally{}}
		t[r.ToolGroup] = group
	}
	group.add(r)
	group.lanes[r.LaneID] = true

	leaf, ok := group.tools[r.ToolLeaf]
	if !ok {
		leaf = &leafTally{tool: r.Tool, lanes: map[string]bool{}}
		group.tools[r.ToolLeaf] = leaf
	}
	leaf.add(r)
	leaf.lanes[r.LaneID] = true
}

// render orders the breakdown for a legend read top down: most calls first, and ties broken by name so two sessions
// with the same tools list them the same way.
func (t toolTallies) render() []toolGroupTotal {
	out := make([]toolGroupTotal, 0, len(t))
	for name, group := range t {
		tools := make([]toolTotal, 0, len(group.tools))
		for leafName, leaf := range group.tools {
			tools = append(tools, toolTotal{
				Tool:     leaf.tool,
				Leaf:     leafName,
				Calls:    leaf.calls,
				Seconds:  seconds(leaf.total),
				Lanes:    len(leaf.lanes),
				Errors:   leaf.errors,
				TimedOut: leaf.timedOut,
			})
		}
		sort.Slice(tools, func(i, j int) bool {
			if tools[i].Calls != tools[j].Calls {
				return tools[i].Calls > tools[j].Calls
			}
			return tools[i].Leaf < tools[j].Leaf
		})

		out = append(out, toolGroupTotal{
			Group:    name,
			Class:    group.class,
			Calls:    group.calls,
			Seconds:  seconds(group.total),
			Lanes:    len(group.lanes),
			Errors:   group.errors,
			TimedOut: group.timedOut,
			Tools:    tools,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Calls != out[j].Calls {
			return out[i].Calls > out[j].Calls
		}
		return out[i].Group < out[j].Group
	})
	return out
}

// inLegendOrder renders a tally in the order a legend shows the kinds, leaving out the ones with no rows behind them:
// a lane that never stalled shouldn't imply it might have. A kind with rows but no measurable time stays, because
// "300 tool calls that cost nothing" is worth seeing.
func inLegendOrder(tallies map[timeline.Kind]*tally) []kindTotal {
	out := make([]kindTotal, 0, len(tallies))
	for _, kind := range timeline.Kinds {
		t, ok := tallies[kind]
		if !ok || t.rows == 0 {
			continue
		}
		out = append(out, kindTotal{Kind: string(kind), Seconds: seconds(t.total), Rows: t.rows})
	}
	return out
}

func gapsOrNone(gaps []gapBody) []gapBody {
	if gaps == nil {
		return []gapBody{}
	}
	return gaps
}

// seconds renders a duration for JSON, to the millisecond the transcripts are stamped at.
func seconds(d time.Duration) float64 { return round(d.Seconds()) }

func round(s float64) float64 { return math.Round(s*1000) / 1000 }

// nilable renders an instant the way this API's conventions ask for: UTC when it's known, and null when it isn't. A
// zero date would serialize as year one, which reads as a real timestamp to anything downstream.
func nilable(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	utc := t.UTC()
	return &utc
}
