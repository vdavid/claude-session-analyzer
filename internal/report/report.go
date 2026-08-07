// Package report renders what the engine derived into the JSON both surfaces answer with.
//
// The HTTP API and the CLI's `--json` return the same shapes from the same code, so a caller learns one vocabulary.
// `docs/api.md` is the contract, and changing a tag here means changing that.
//
// Every duration is seconds, as a number, named `seconds` or `...Seconds`. Every instant is RFC 3339 in UTC. A
// timestamp that isn't known is null rather than a zero date, because 99 of the 725 sessions on the machine this was
// built against carry no timestamped record at all and 1 January year 1 would be a lie.
package report

import (
	"math"
	"path/filepath"
	"slices"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/agg"
	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

// SessionList is every session under a root.
type SessionList struct {
	// Root is where the sessions were read from, so a caller can say so without guessing.
	Root     string    `json:"root"`
	Sessions []Session `json:"sessions"`
	// Totals cover every session the filters let through, not only the ones a limit showed.
	Totals SessionListTotals `json:"totals"`
}

type SessionListTotals struct {
	Sessions  int   `json:"sessions"`
	Subagents int   `json:"subagents"`
	Bytes     int64 `json:"bytes"`
}

// Session is one session as a list and a session header show it. It costs two reads of the lead transcript's ends, so
// it stays cheap on a root holding thousands of sessions.
type Session struct {
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

// Timeline is one session's rows plus everything a chart needs already summed.
type Timeline struct {
	Session Session `json:"session"`
	Totals  Totals  `json:"totals"`
	Lanes   []Lane  `json:"lanes"`
	// Rows is every activity row, sorted by start. It's empty when the caller asked for no rows, and Totals.Rows still
	// says how many there were.
	Rows []Row `json:"rows"`
}

// Totals is the aggregation a browser would otherwise do over 15,000 rows.
type Totals struct {
	// From and Until bracket every lane, the subagents included, so they can run slightly wider than the session's own
	// start and end. Both are null on a session whose records carry no timestamp: a zero date renders as year one,
	// which is a lie a reader has no way to spot.
	From  *time.Time `json:"from"`
	Until *time.Time `json:"until"`
	// WallClockSeconds is From to Until: how long the session took. LaneTimeSeconds is every lane's rows added up,
	// which is larger whenever lanes ran at the same time. They answer different questions and are never the same
	// number in a multi-agent session; don't present one as the other.
	WallClockSeconds float64 `json:"wallClockSeconds"`
	LaneTimeSeconds  float64 `json:"laneTimeSeconds"`
	// NetSeconds and ActiveSeconds are the two rungs below lane time, each the one above minus something. Net is lane
	// time minus waiting for a person and waiting for a teammate (`Kind.IsSomeoneElsesClock`), so it's the agent time
	// the session actually cost: a wait on a teammate is already that teammate's own lane time, and counting it here
	// counts the same work twice. Active is net minus the gaps net keeps (stalls, API errors, background-task waits,
	// unknown waits), so it answers a different question, "how much was producing", and neither replaces the other. On
	// the reference session net holds a 6h15m stall that active doesn't. The arithmetic is in `docs/api.md`.
	NetSeconds    float64 `json:"netSeconds"`
	ActiveSeconds float64 `json:"activeSeconds"`
	Rows          int     `json:"rows"`
	Lanes         int     `json:"lanes"`
	// ByKind is the pie: one entry per activity kind that took time, in legend order.
	ByKind []KindTotal `json:"byKind"`
	// ByTool is the tool breakdown, biggest first: one entry per group, each holding the exact tools inside it.
	ByTool []ToolGroupTotal `json:"byTool"`
}

type KindTotal struct {
	Kind    string  `json:"kind"`
	Seconds float64 `json:"seconds"`
	Rows    int     `json:"rows"`
}

// ToolGroupTotal is one slice of the tool breakdown: everything a session did with one tool, at the level a reader asks
// about. `Bash` is a dozen tools wearing one name and an MCP server arrives as one tool per method, so the group is
// what the engine's ToolID calls it: `Bash (git)`, `codegraph (MCP)`, `Read`.
//
// The counts are calls, not rows. Every call has one row for the agent composing it and one for the tool running, and
// only the second is counted, so Calls is the number of times the tool was actually used.
//
// The time is three numbers, because a call spends it on three clocks and adding them together hides which. See
// Seconds, ComposingSeconds, and StalledSeconds below, and `docs/api.md`.
type ToolGroupTotal struct {
	Group string `json:"group"`
	// Class is what kind of work the group does, which is what colours it. Every tool in a group shares it.
	Class string `json:"class"`
	Calls int    `json:"calls"`
	// Seconds is the tool running: its own wall clock, including anything the tool waited on. The other two clocks are
	// out of it.
	Seconds float64 `json:"seconds"`
	// ComposingSeconds is the agent's clock writing the calls. It inverts per tool rather than being a rounding error:
	// on the reference session the `Edit` group spent 2.24 h composing against 0.03 h running, because the model
	// streams the whole diff as the call's arguments, while `Bash (checker)` spent 0.40 h composing against 7.86 h
	// running (verified 2026-08-08).
	ComposingSeconds float64 `json:"composingSeconds"`
	// StalledSeconds is the time of calls that came back far too late to have been running, which is a suspended agent
	// rather than a slow tool. Left out when nothing stalled. On the reference session `Bash (file write)` looks
	// pathological at 6.36 h over 69 calls until this splits one stalled `rm` of 6.26 h off the other 68, which average
	// 5.2 s. The stall is still one of Calls: it was a call.
	StalledSeconds float64 `json:"stalledSeconds,omitempty"`
	// Lanes is how many lanes made one of these calls, which answers "who used this" without reading the rows. It's
	// counted per group rather than added up from the tools, because one lane calling two of them is still one lane.
	Lanes    int `json:"lanes"`
	Errors   int `json:"errors,omitempty"`
	TimedOut int `json:"timedOut,omitempty"`
	// Tools are the exact things that ran inside the group, biggest first.
	Tools []ToolTotal `json:"tools"`
}

// ToolTotal is one exact tool inside a group. The three clocks mean what they do on the group.
type ToolTotal struct {
	// Tool is the raw name the harness used, so a reader can grep a transcript for it. Leaf is the part that varies
	// inside the group: an MCP method, or the program a Bash call ran.
	Tool             string  `json:"tool"`
	Leaf             string  `json:"leaf"`
	Calls            int     `json:"calls"`
	Seconds          float64 `json:"seconds"`
	ComposingSeconds float64 `json:"composingSeconds"`
	StalledSeconds   float64 `json:"stalledSeconds,omitempty"`
	Lanes            int     `json:"lanes"`
	Errors           int     `json:"errors,omitempty"`
	TimedOut         int     `json:"timedOut,omitempty"`
}

// Lane is one swimlane: when the lane was alive, where its holes are, and what it spent its time on.
type Lane struct {
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
	ByKind  []KindTotal `json:"byKind"`
	// Gaps are the stretches the lane produced nothing: waiting, and stalled. Drawing a lane as one solid bar between
	// From and Until would claim it was busy the whole time.
	Gaps []Gap `json:"gaps"`
}

type Gap struct {
	From    time.Time `json:"from"`
	Until   time.Time `json:"until"`
	Seconds float64   `json:"seconds"`
	Kind    string    `json:"kind"`
	Info    string    `json:"info,omitempty"`
}

// Row is one activity row. It carries everything the derivation knows, which is more than the CSV's six columns.
type Row struct {
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
	// without the browser re-deriving a rule that lives in the engine. The leaf is left out: nothing filters by it, and
	// the breakdown already carries the leaves it needs.
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

// ForSession renders one session's metadata.
func ForSession(s session.Summary) Session {
	body := Session{
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
		body.Seconds = Seconds(s.Duration())
	}
	return body
}

// ForTimeline aggregates a derived timeline into what a chart draws. The totals come from one cube, which is the same
// sum a cached digest and a `stats` query are made of.
func ForTimeline(sum session.Summary, tl *timeline.Timeline, withRows bool) Timeline {
	cube := agg.Build(tl, agg.Options{})

	body := Timeline{
		Session: ForSession(sum),
		Totals:  TotalsFrom(cube.Cells()),
		Lanes:   make([]Lane, 0, len(tl.Lanes)),
		Rows:    []Row{},
	}
	body.Totals.From = nilable(tl.First)
	body.Totals.Until = nilable(tl.Last)
	body.Totals.WallClockSeconds = Seconds(tl.Duration())
	body.Totals.Lanes = len(tl.Lanes)

	// The gaps and the rows are the two things a cube can't hold, so they're the one pass over the rows.
	gaps := make(map[string][]Gap, len(tl.Lanes))
	if withRows {
		body.Rows = make([]Row, 0, len(tl.Rows))
	}
	for _, r := range tl.Rows {
		if r.Kind.IsGap() {
			gaps[r.LaneID] = append(gaps[r.LaneID], Gap{
				From:    r.From.UTC(),
				Until:   r.Until.UTC(),
				Seconds: Seconds(r.Duration()),
				Kind:    string(r.Kind),
				Info:    r.Info,
			})
		}
		if withRows {
			body.Rows = append(body.Rows, toRow(r))
		}
	}

	byLane := agg.RollUp(cube.Cells(), agg.ByLane|agg.ByKind)
	for _, lane := range tl.Lanes {
		cells := forLane(byLane, lane.ID)
		body.Lanes = append(body.Lanes, Lane{
			ID:         lane.ID,
			Name:       lane.Name,
			IsLead:     lane.IsLead,
			WorkflowID: lane.WorkflowID,
			Model:      lane.Model,
			Color:      lane.Color,
			From:       lane.First.UTC(),
			Until:      lane.Last.UTC(),
			Seconds:    Seconds(lane.Duration()),
			Rows:       agg.Sum(cells).Rows,
			ByKind:     KindTotals(cells),
			Gaps:       gapsOrNone(gaps[lane.ID]),
		})
	}
	return body
}

// TotalsFrom sums a set of cells into the totals block, leaving the fields only a timeline knows (its span, its lane
// count) for the caller to fill in. A `stats` answer over many sessions uses the same function.
func TotalsFrom(cells []agg.Cell) Totals {
	whole := agg.Sum(cells)
	byKind := agg.RollUp(cells, agg.ByKind)

	// The ladder, one pass: net drops the clocks that belong to someone else, and active drops the gaps net keeps.
	var net, active time.Duration
	for _, c := range byKind {
		kind := timeline.Kind(c.Kind)
		if kind.IsSomeoneElsesClock() {
			continue
		}
		net += c.Duration
		if !kind.IsGap() {
			active += c.Duration
		}
	}

	return Totals{
		LaneTimeSeconds: Seconds(whole.Duration),
		NetSeconds:      Seconds(net),
		ActiveSeconds:   Seconds(active),
		Rows:            whole.Rows,
		ByKind:          KindTotals(cells),
		ByTool:          ToolTotals(cells),
	}
}

// KindTotals renders the pie in the order a legend shows the kinds, leaving out the ones with no rows behind them: a
// lane that never stalled shouldn't imply it might have. A kind with rows but no measurable time stays, because "300
// tool calls that cost nothing" is worth seeing.
func KindTotals(cells []agg.Cell) []KindTotal {
	byKind := make(map[string]agg.Cell, len(timeline.Kinds))
	for _, c := range agg.RollUp(cells, agg.ByKind) {
		byKind[c.Kind] = c
	}

	out := make([]KindTotal, 0, len(byKind))
	for _, kind := range timeline.Kinds {
		c, ok := byKind[string(kind)]
		if !ok || c.Rows == 0 {
			continue
		}
		out = append(out, KindTotal{Kind: string(kind), Seconds: Seconds(c.Duration), Rows: c.Rows})
	}
	return out
}

// ToolTotals renders the tool breakdown, most calls first, ties broken by name so two sessions with the same tools list
// them the same way.
//
// A call's time is split three ways rather than added together. `agg.ToolRuns` drops the rows where an agent was
// composing a call, which would otherwise inflate what the tool cost, and those rows come back as ComposingSeconds; the
// stalls inside the runs come out of Seconds and back as StalledSeconds. All three carry the group's name, so the three
// together account for every row the grouping rule put in the group.
func ToolTotals(cells []agg.Cell) []ToolGroupTotal {
	runs := agg.ToolRuns(cells)
	const leafDims = agg.ByGroup | agg.ByLeaf | agg.ByTool | agg.ByClass
	const groupDims = agg.ByGroup | agg.ByClass
	byLeaf, byGroup := otherClocks(cells, leafDims), otherClocks(cells, groupDims)

	leaves := map[string][]agg.Cell{}
	for _, c := range agg.RollUp(runs, leafDims) {
		leaves[c.Group] = append(leaves[c.Group], c)
	}

	out := make([]ToolGroupTotal, 0, len(leaves))
	for _, group := range agg.RollUp(runs, groupDims) {
		tools := make([]ToolTotal, 0, len(leaves[group.Group]))
		for _, leaf := range leaves[group.Group] {
			other := byLeaf[leaf.Key]
			tools = append(tools, ToolTotal{
				Tool:             leaf.Tool,
				Leaf:             leaf.Leaf,
				Calls:            leaf.Calls,
				Seconds:          Seconds(leaf.Duration - other.stalled),
				ComposingSeconds: Seconds(other.composing),
				StalledSeconds:   Seconds(other.stalled),
				Lanes:            leaf.Lanes,
				Errors:           leaf.Errors,
				TimedOut:         leaf.TimedOut,
			})
		}
		slices.SortFunc(tools, func(a, b ToolTotal) int {
			if a.Calls != b.Calls {
				return b.Calls - a.Calls
			}
			if a.Leaf < b.Leaf {
				return -1
			}
			return 1
		})

		other := byGroup[group.Key]
		out = append(out, ToolGroupTotal{
			Group:            group.Group,
			Class:            group.Class,
			Calls:            group.Calls,
			Seconds:          Seconds(group.Duration - other.stalled),
			ComposingSeconds: Seconds(other.composing),
			StalledSeconds:   Seconds(other.stalled),
			Lanes:            group.Lanes,
			Errors:           group.Errors,
			TimedOut:         group.TimedOut,
			Tools:            tools,
		})
	}
	slices.SortFunc(out, func(a, b ToolGroupTotal) int {
		if a.Calls != b.Calls {
			return b.Calls - a.Calls
		}
		if a.Group < b.Group {
			return -1
		}
		return 1
	})
	return out
}

// clocks is the time a tool group holds that isn't the tool running.
type clocks struct {
	composing time.Duration
	stalled   time.Duration
}

// otherClocks sums the composing and stalled time per key, at the same grain the runs beside it are rolled up to, so a
// group and a leaf each look their own two numbers up rather than re-deriving them.
func otherClocks(cells []agg.Cell, dims agg.Dim) map[agg.Key]clocks {
	out := map[agg.Key]clocks{}
	for _, c := range agg.RollUp(agg.Composing(cells), dims) {
		entry := out[c.Key]
		entry.composing += c.Duration
		out[c.Key] = entry
	}
	for _, c := range agg.RollUp(agg.Stalls(cells), dims) {
		entry := out[c.Key]
		entry.stalled += c.Duration
		out[c.Key] = entry
	}
	return out
}

func forLane(cells []agg.Cell, lane string) []agg.Cell {
	var out []agg.Cell
	for _, c := range cells {
		if c.Lane == lane {
			out = append(out, c)
		}
	}
	return out
}

func toRow(r timeline.Row) Row {
	return Row{
		From:       r.From.UTC(),
		Until:      r.Until.UTC(),
		Seconds:    Seconds(r.Duration()),
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

func gapsOrNone(gaps []Gap) []Gap {
	if gaps == nil {
		return []Gap{}
	}
	return gaps
}

// Seconds renders a duration for JSON, to the millisecond the transcripts are stamped at.
func Seconds(d time.Duration) float64 { return math.Round(d.Seconds()*1000) / 1000 }

// nilable renders an instant the way this package's conventions ask for: UTC when it's known, and null when it isn't. A
// zero date would serialize as year one, which reads as a real timestamp to anything downstream.
func nilable(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	utc := t.UTC()
	return &utc
}
