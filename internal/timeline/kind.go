package timeline

import (
	"fmt"
	"time"
)

// Kind is what an agent was doing during a row. The names are the ones the CSV and the UI show, so they're phrased for
// a reader rather than for a compiler.
type Kind string

const (
	// KindThinking is the model producing a thinking block. The span starts when the previous block finished, so it
	// includes API queue time and prompt processing, not only reasoning. Say so wherever it's shown.
	KindThinking Kind = "thinking"
	// KindWriting is the agent composing prose for its caller: a text block.
	KindWriting Kind = "writing"
	// KindToolCall is the agent composing a tool call, from the previous block until the tool_use block finished
	// streaming. It's often a fraction of a second, and zero when the call shared a record with the block before it.
	KindToolCall Kind = "tool call"
	// KindToolExecution is the tool running, from the tool_use block until its result came back. This is the honest
	// wall clock of the tool, including anything the tool waited on.
	KindToolExecution Kind = "tool execution"
	// The four waits below are idle gaps: the lane produced nothing, and the kind says what it was idle on. They're
	// separate kinds rather than one, because "71 hours of waiting" answers nothing while "41 hours on a person, 16 on
	// background tasks, 14 on teammates" answers the question that gets asked. Each one is named for what was waited
	// on rather than for who waited, so a subagent's wait and a lead's wait land in the same bucket.

	// KindWaitingForPerson is a gap a person closed: a prompt typed or queued, or an answer to a question the agent
	// asked.
	KindWaitingForPerson Kind = "waiting for a person"
	// KindWaitingForTeammate is a gap another agent closed: its message, read off the envelope the harness wraps it in,
	// or the notification of a background task it had left running while it was still alive. See attributeTaskWaits.
	KindWaitingForTeammate Kind = "waiting for a teammate"
	// KindWaitingForTask is a gap a background task's notification closed: a build, a test suite, or a poll loop the
	// agent had left running. Only where the task wasn't a live teammate's, which is the wait above.
	KindWaitingForTask Kind = "waiting for a background task"
	// KindWaitingUnknown is a gap with nothing on record saying what it was waiting for. The lane went quiet and later
	// produced something, and no input, message, or notification sits between the two.
	KindWaitingUnknown Kind = "waiting, reason unknown"
	// KindAPIError is a request the API didn't answer: an outage, a rate limit, an expired login, or a refusal. The
	// span is the harness retrying, which is time the session lost through no fault of the agent, and long enough to be
	// filed as idle time if nothing named it. See apierror.go.
	KindAPIError Kind = "API error"
	// KindStalled is a tool result that arrived far too late for what the tool was doing. The agent was suspended, not
	// working. See stall.go for how far is too far and why.
	KindStalled Kind = "stalled"
	// KindCompacting is the harness compacting the context. It's real wall clock that belongs to neither the agent nor
	// a tool, and a single compaction runs into minutes, so it gets its own kind rather than inflating thinking or
	// hiding inside waiting. Its duration comes from the compaction's own record, because the timestamps around it
	// disagree.
	KindCompacting Kind = "compacting"
)

// Kinds lists every activity kind, in the order a legend should show them: what the agent did, then what it waited
// for, then the three ways time goes somewhere else.
var Kinds = []Kind{
	KindThinking, KindWriting, KindToolCall, KindToolExecution,
	KindWaitingForPerson, KindWaitingForTeammate, KindWaitingForTask, KindWaitingUnknown,
	KindAPIError, KindStalled, KindCompacting,
}

// IsWaiting says the kind is one of the waits, whatever it was waiting on. Code that means "idle" should ask this
// rather than list the four kinds, so a fifth one can't be missed.
func (k Kind) IsWaiting() bool {
	switch k {
	case KindWaitingForPerson, KindWaitingForTeammate, KindWaitingForTask, KindWaitingUnknown:
		return true
	}
	return false
}

// IsGap says the lane produced nothing during the row: it was waiting, suspended, or held up by the API. These are the
// holes in a swimlane, and drawing a lane solid across one would claim it was busy when it wasn't.
func (k Kind) IsGap() bool { return k.IsWaiting() || k == KindStalled || k == KindAPIError }

// IsSomeoneElsesClock says the row is time that belongs to someone other than this lane's agent: a person typing, or a
// teammate working. It's the one thing net time takes out of lane time, and it exists as a predicate because the two
// kinds are picked out for a reason rather than as a list:
//
//   - A wait on a teammate is already counted as that teammate's own lane time, so a total that keeps it counts the
//     same work twice.
//   - A wait on a person was never agent time at all.
//
// The other two waits are this lane's own clock going nowhere, so net keeps them and only active takes them out. Ask
// this rather than naming the two kinds, so a fifth wait can't be missed.
func (k Kind) IsSomeoneElsesClock() bool {
	switch k {
	case KindWaitingForPerson, KindWaitingForTeammate:
		return true
	}
	return false
}

// Row is one stretch of one lane's wall clock.
type Row struct {
	From  time.Time
	Until time.Time
	// Agent is the lane's label, which is what a reader recognises. LaneID is what code should match on.
	Agent  string
	LaneID string
	Kind   Kind
	// Info is the "Extra info" column: a short phrase saying what this row was about.
	Info string

	// Tool is the tool a tool call, tool execution, or stalled row is about, empty otherwise.
	Tool string
	// Class is what kind of work the tool was doing. Only meaningful alongside Tool.
	Class ToolClass
	// ToolGroup and ToolLeaf are how a breakdown names the call, and are only meaningful alongside Tool. `Bash` is a
	// dozen tools wearing one name, so the group splits it by what it was doing and the leaf names the program:
	// `Bash (git)` and `git commit`. See ToolID.
	ToolGroup string
	ToolLeaf  string

	// Overlapped marks a row that ran concurrently with a sibling in the same lane, which happens when one response
	// fires several tool calls at once. These are the only rows that break the tiling.
	Overlapped bool
	// TimedOut marks a tool execution the harness cut short at its timeout.
	TimedOut bool
	// IsError marks a tool result the tool reported as a failure.
	IsError bool

	// Line is the 1-based line in the lane's transcript that closed this row, for tracing a row back to its source.
	Line int
}

// Duration is how long the row lasted. It's never negative: the derivation clamps a backwards timestamp to zero.
func (r Row) Duration() time.Duration { return r.Until.Sub(r.From) }

// IsToolRun says the row is the stretch a tool call actually ran for, whatever the derivation ended up calling it: a
// tool execution, a stall, or a wait on the person the tool asked. Every call has exactly one of these and exactly one
// KindToolCall row beside it, so counting these counts calls. Ask this rather than listing kinds, so a new verdict
// can't be missed.
func (r Row) IsToolRun() bool { return r.Tool != "" && r.Kind != KindToolCall }

// LaneSpan is when a lane was alive, for drawing a swimlane and for saying who was around during a wait.
type LaneSpan struct {
	ID     string
	Name   string
	IsLead bool
	// WorkflowID names the workflow that spawned the lane, empty when the session spawned it directly.
	WorkflowID string
	Model      string
	// Color is the lane's colour as the terminal showed it, often empty.
	Color string
	First time.Time
	Last  time.Time
	// Rows counts the lane's rows, so a caller can slice the timeline without scanning it.
	Rows int
}

// Duration is how long the lane was alive.
func (l LaneSpan) Duration() time.Duration { return l.Last.Sub(l.First) }

// Timeline is a whole session's rows, plus where each lane sat in time.
type Timeline struct {
	SessionID string
	// Rows hold every lane's rows, sorted by start time, then by lane order, then by the order they were derived.
	Rows []Row
	// Lanes are in session order: the lead first, then the subagents.
	Lanes []LaneSpan
	// First and Last bracket the whole session.
	First time.Time
	Last  time.Time
}

// Duration is the session's wall clock from its first record to its last.
func (t *Timeline) Duration() time.Duration { return t.Last.Sub(t.First) }

// FormatDuration renders a duration the way the Info column does: compact, and never more precise than it's worth.
func FormatDuration(d time.Duration) string {
	switch {
	case d < 0:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}
