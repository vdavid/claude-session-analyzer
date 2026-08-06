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
	// KindWaiting is an idle gap: the lane produced nothing, because it was waiting on a person, on a teammate, or on
	// a background task.
	KindWaiting Kind = "waiting"
	// KindStalled is a tool result that arrived far too late for what the tool was doing. The agent was suspended, not
	// working. See stall.go for how far is too far and why.
	KindStalled Kind = "stalled"
	// KindCompacting is the harness compacting the context. It's real wall clock that belongs to neither the agent nor
	// a tool, and a single compaction runs into minutes, so it gets its own kind rather than inflating thinking or
	// hiding inside waiting. Its duration comes from the compaction's own record, because the timestamps around it
	// disagree.
	KindCompacting Kind = "compacting"
)

// Kinds lists every activity kind, in the order a legend should show them.
var Kinds = []Kind{
	KindThinking, KindWriting, KindToolCall, KindToolExecution, KindWaiting, KindStalled, KindCompacting,
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
