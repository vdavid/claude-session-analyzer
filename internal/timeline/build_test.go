package timeline

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// base is the instant every hand-built fixture counts seconds from. It's a round UTC time so a failure message reads
// as an offset rather than as a date.
var base = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

// at turns an offset in seconds into a timestamp.
func at(sec float64) time.Time {
	return base.Add(time.Duration(sec * float64(time.Second)))
}

// laneOf builds a lane out of records, stamping each one at the offset it's paired with. It's the shorthand every
// derivation test writes its input in.
type laneOf struct {
	lane *session.Lane
}

func newLane(name string, lead bool) *laneOf {
	return &laneOf{lane: &session.Lane{ID: name, Name: name, IsLead: lead}}
}

// add stamps a record and appends it. Later records may be stamped earlier than earlier ones: transcripts do that.
func (b *laneOf) add(sec float64, rec *transcript.Record) *laneOf {
	rec.Timestamp = at(sec)
	rec.Line = len(b.lane.Records) + 1
	b.lane.Records = append(b.lane.Records, rec)
	return b
}

func (b *laneOf) done() *session.Lane { return b.lane }

// assistantRec is one assistant record holding the blocks of one response. Older transcripts pack several blocks into
// one record, so a test can too.
func assistantRec(blocks ...transcript.Block) *transcript.Record {
	return &transcript.Record{Type: transcript.TypeAssistant, Blocks: blocks}
}

func thinkingBlock(text string) transcript.Block {
	return transcript.Block{Type: transcript.BlockThinking, Text: text, TextBytes: len(text), Signature: "sig"}
}

func textBlock(text string) transcript.Block {
	return transcript.Block{Type: transcript.BlockText, Text: text, TextBytes: len(text)}
}

// toolUseBlock builds a call. Pass input as key, value pairs; each value is encoded as a JSON string, which is what
// every input field this package reads is.
func toolUseBlock(id, name string, input ...string) transcript.Block {
	b := transcript.Block{Type: transcript.BlockToolUse, ToolUseID: id, ToolName: name}
	if len(input) > 0 {
		b.Input = map[string]json.RawMessage{}
		for i := 0; i+1 < len(input); i += 2 {
			raw, err := json.Marshal(input[i+1])
			if err != nil {
				panic(err)
			}
			b.Input[input[i]] = raw
			b.InputBytes += len(raw)
		}
	}
	return b
}

// withNumber adds a numeric input field, which is how a Bash timeout arrives.
func withNumber(b transcript.Block, key string, value int) transcript.Block {
	if b.Input == nil {
		b.Input = map[string]json.RawMessage{}
	}
	b.Input[key] = json.RawMessage(jsonNumber(value))
	return b
}

func jsonNumber(v int) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// toolResultRec is the user record carrying one tool's result.
func toolResultRec(id, text string) *transcript.Record {
	return &transcript.Record{
		Type:   transcript.TypeUser,
		Blocks: []transcript.Block{{Type: transcript.BlockToolResult, ToolUseID: id, Text: text, TextBytes: len(text)}},
	}
}

// withResultPayload attaches the structured `toolUseResult` payload, which is where a Bash timeout is recorded.
func withResultPayload(rec *transcript.Record, payload string) *transcript.Record {
	rec.ToolUseResult = json.RawMessage(payload)
	rec.ToolUseResultBytes = len(payload)
	return rec
}

func promptRec(text string) *transcript.Record {
	return &transcript.Record{Type: transcript.TypeUser, Prompt: text, PromptBytes: len(text)}
}

func attachmentRec(kind string) *transcript.Record {
	return &transcript.Record{Type: transcript.TypeAttachment, Attachment: kind}
}

func systemRec(subtype string) *transcript.Record {
	return &transcript.Record{Type: transcript.TypeSystem, System: &transcript.SystemInfo{Subtype: subtype}}
}

func compactRec(d time.Duration, pre, post int) *transcript.Record {
	return &transcript.Record{Type: transcript.TypeSystem, System: &transcript.SystemInfo{
		Subtype: "compact_boundary",
		Compact: &transcript.CompactInfo{Trigger: "manual", PreTokens: pre, PostTokens: post, Duration: d},
	}}
}

func queueRec(op, content string) *transcript.Record {
	return &transcript.Record{Type: transcript.TypeQueueOperation, Queue: &transcript.QueueInfo{
		Operation: op, Content: content,
	}}
}

// sessionOf wraps lanes into a session, the lead first.
func sessionOf(lanes ...*session.Lane) *session.Session {
	s := &session.Session{}
	s.ID = "test-session"
	s.Lanes = lanes
	return s
}

// rowSummary renders a row as "kind from-until info", which is what a failing assertion is easiest to read as.
func rowSummary(r Row) string {
	return string(r.Kind) + " " + offset(r.From) + "-" + offset(r.Until) + " " + r.Info
}

func offset(t time.Time) string {
	return FormatDuration(t.Sub(base))
}

// requireKinds checks the kinds of a lane's rows in order, and reports the whole lane when they differ.
func requireKinds(t *testing.T, rows []Row, want ...Kind) {
	t.Helper()
	got := make([]Kind, 0, len(rows))
	for _, r := range rows {
		got = append(got, r.Kind)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d:\n%s", len(got), len(want), dump(rows))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d is %q, want %q:\n%s", i, got[i], want[i], dump(rows))
		}
	}
}

func dump(rows []Row) string {
	out := ""
	for i, r := range rows {
		out += "  " + string(rune('0'+i%10)) + " " + rowSummary(r) + "\n"
	}
	return out
}

// apiErrorRec is the record the harness writes when the API doesn't answer: an assistant record carrying prose for the
// person at the terminal, plus the typed fields that say it wasn't the agent writing. A status of zero is a record
// that carried none, which is 76 of the corpus's 245.
func apiErrorRec(kind string, status int, text string) *transcript.Record {
	rec := &transcript.Record{
		Type:     transcript.TypeAssistant,
		APIError: &transcript.APIError{Kind: kind, Status: status},
	}
	if text != "" {
		rec.Blocks = []transcript.Block{textBlock(text)}
	}
	return rec
}
