package transcript

import (
	"encoding/json"
	"time"
)

// RecordType is a transcript line's `type` field. Values other than the constants below appear over time and are
// skipped by the reader, so a RecordType outside this set never reaches a caller.
type RecordType string

const (
	TypeAssistant      RecordType = "assistant"
	TypeUser           RecordType = "user"
	TypeAttachment     RecordType = "attachment"
	TypeSystem         RecordType = "system"
	TypeQueueOperation RecordType = "queue-operation"

	// The three title types below carry no timestamp and get rewritten on most turns, so the last one in a file wins.
	// A custom title is what a person set; an AI title and an agent name are generated.
	TypeCustomTitle RecordType = "custom-title"
	TypeAITitle     RecordType = "ai-title"
	TypeAgentName   RecordType = "agent-name"
)

// BlockType is a content block's kind inside an assistant or user message.
type BlockType string

const (
	BlockThinking   BlockType = "thinking"
	BlockText       BlockType = "text"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
	BlockImage      BlockType = "image"
)

// Record is one decoded transcript line. Fields that don't apply to the record's type stay zero.
type Record struct {
	Type RecordType
	// Line is the 1-based line number in the transcript, for diagnostics.
	Line int

	UUID       string
	ParentUUID string
	// RequestID groups the blocks that came back from one API response. Assistant records only.
	RequestID string
	// SessionID is the lead session's id, even on a subagent record, so it can't tell lanes apart.
	SessionID string
	// AgentID is set on subagent records and matches the transcript file's name minus its `agent-` prefix.
	AgentID string

	// Timestamp is when the record finished being written, in UTC. Zero on record types that carry none.
	Timestamp time.Time
	// IsSidechain marks a record written in a subagent lane.
	IsSidechain bool
	// IsMeta marks a harness-injected record, not something a person typed.
	IsMeta bool

	CWD       string
	GitBranch string
	Version   string

	// Model is the model that produced an assistant record.
	Model string
	// Blocks holds the message's content blocks, in order. Assistant and user records.
	Blocks []Block
	// Prompt is a user record's text when its content is a bare string, which means a person typed it.
	Prompt string
	// Usage carries token counts on an assistant record. Blocks before the last of a request hold partial counts.
	Usage *Usage

	// Title is the text of a custom-title, ai-title, or agent-name record.
	Title string

	// Attachment is an attachment record's `attachment.type`.
	Attachment string
	// System carries a system record's subtype and payload.
	System *SystemInfo
	// Queue carries a queue-operation record's payload.
	Queue *QueueInfo

	// ToolUseResult is a tool result's structured payload, kept raw because its shape is per-tool. Nil when the record
	// has none, and also when the payload was larger than Options.MaxValueBytes, which ToolUseResultBytes then reports.
	ToolUseResult json.RawMessage
	// ToolUseResultBytes is the payload's size on the wire, whether or not it was kept.
	ToolUseResultBytes int
}

// Block is one content block of a message.
type Block struct {
	Type BlockType

	// Text is a text block's prose, a thinking block's reasoning, or a tool result's content flattened to text.
	// Thinking text is empty in almost every transcript, so treat a non-empty value as a bonus rather than a given.
	Text string
	// TextBytes is Text's length on the wire, which is larger than len(Text) when Truncated is set.
	TextBytes int
	// Truncated says Text was cut at Options.MaxValueBytes.
	Truncated bool

	// Signature is a thinking block's signature, the only part of it that always survives.
	Signature string

	// ToolUseID is a tool_use block's `id` or a tool_result block's `tool_use_id`. Pair the two on it.
	ToolUseID string
	// ToolName is the tool a tool_use block calls.
	ToolName string
	// Input holds a tool_use block's arguments, one raw JSON value per key. Keys whose value was larger than
	// Options.MaxValueBytes are dropped and named in InputElided, so what's here is always valid JSON.
	Input map[string]json.RawMessage
	// InputElided names the input keys dropped for size, sorted.
	InputElided []string
	// InputBytes is the input object's size on the wire.
	InputBytes int

	// IsError marks a tool result the tool reported as a failure.
	IsError bool
}

// Usage is an assistant record's token accounting.
type Usage struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

// SystemInfo is a system record's payload.
type SystemInfo struct {
	Subtype string
	Content string
	// Duration is a turn_duration record's `durationMs`.
	Duration     time.Duration
	MessageCount int
	// Compact is set on a compact_boundary record.
	Compact *CompactInfo
}

// CompactInfo describes one compaction. Its Duration is the honest source for the gap compaction leaves in a lane,
// because the records around it are stamped inconsistently.
type CompactInfo struct {
	Trigger    string
	PreTokens  int
	PostTokens int
	Duration   time.Duration
}

// QueueInfo is a queue-operation record's payload. An `enqueue` timestamps when a person typed, not when the agent
// got around to reading it.
type QueueInfo struct {
	Operation string
	Content   string
}
