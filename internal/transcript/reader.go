package transcript

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Unlimited turns off Options.MaxValueBytes.
const Unlimited = -1

// DefaultMaxValueBytes is the cap a zero Options uses. It's generous enough to keep a tool call's command line and the
// head of its output, and small enough that a whole session stays comfortably in memory.
const DefaultMaxValueBytes = 8 << 10

// Options tunes a Reader.
type Options struct {
	// MaxValueBytes caps how much of any single payload is kept. Zero means DefaultMaxValueBytes, Unlimited keeps
	// everything.
	MaxValueBytes int
}

func (o Options) cap() int {
	if o.MaxValueBytes == 0 {
		return DefaultMaxValueBytes
	}
	return o.MaxValueBytes
}

// Stats reports what a Reader saw, so a caller can account for every line it fed in. Decoded, Skipped, Blank, and
// Malformed always add up to Lines.
type Stats struct {
	Lines   int
	Decoded int
	// Skipped counts lines whose `type` isn't one this package decodes. New types appear over time, so this being
	// non-zero is normal.
	Skipped      int
	SkippedTypes map[string]int
	Blank        int
	// Malformed counts lines that aren't decodable JSON, which a transcript truncated mid-write ends with.
	Malformed          int
	FirstMalformedLine int
	FirstMalformedErr  error
	LongestLine        int
}

// Reader streams records off a transcript. It retains nothing between calls, so it runs in constant memory over
// transcripts of any size, and every record it hands out is freshly allocated and safe to keep.
type Reader struct {
	br    *bufio.Reader
	opts  Options
	buf   []byte
	line  int
	rec   *Record
	err   error
	stats Stats
}

// NewReader starts a reader over r.
func NewReader(r io.Reader, opts Options) *Reader {
	return &Reader{
		br:    bufio.NewReaderSize(r, 64<<10),
		opts:  opts,
		stats: Stats{SkippedTypes: map[string]int{}},
	}
}

// Next advances to the next decodable record and reports whether there was one. Lines this package doesn't decode, and
// lines that aren't valid JSON, are counted in Stats and stepped over.
func (r *Reader) Next() bool {
	for {
		line, err := r.readLine()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				r.err = err
			}
			r.rec = nil
			return false
		}

		r.line++
		r.stats.Lines++
		if len(line) > r.stats.LongestLine {
			r.stats.LongestLine = len(line)
		}

		trimmed := trimSpace(line)
		if len(trimmed) == 0 {
			r.stats.Blank++
			continue
		}

		var wire wireRecord
		if err := json.Unmarshal(trimmed, &wire); err != nil {
			r.stats.Malformed++
			if r.stats.FirstMalformedLine == 0 {
				r.stats.FirstMalformedLine = r.line
				r.stats.FirstMalformedErr = err
			}
			continue
		}

		if !isDecodedType(wire.Type) {
			r.stats.Skipped++
			r.stats.SkippedTypes[wire.Type]++
			continue
		}

		r.rec = wire.toRecord(r.line, r.opts.cap())
		r.stats.Decoded++
		return true
	}
}

// Record returns the record Next just decoded.
func (r *Reader) Record() *Record { return r.rec }

// Err returns the first read error, if any. A malformed line isn't one: it's counted in Stats.
func (r *Reader) Err() error { return r.err }

// Stats returns what the reader has seen so far.
func (r *Reader) Stats() Stats { return r.stats }

// readLine returns the next line without its terminator. Transcript lines run past a megabyte, so it grows a buffer
// rather than working to a fixed ceiling the way bufio.Scanner does. The returned slice is only valid until the next
// call.
func (r *Reader) readLine() ([]byte, error) {
	r.buf = r.buf[:0]
	for {
		chunk, err := r.br.ReadSlice('\n')
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			r.buf = append(r.buf, chunk...)
			continue
		case err != nil:
			if len(chunk) == 0 && len(r.buf) == 0 {
				return nil, err
			}
			// A final line with no trailing newline. The error comes back on the next call.
			r.buf = append(r.buf, chunk...)
			return r.buf, nil
		}
		chunk = chunk[:len(chunk)-1] // drop the newline
		if len(r.buf) == 0 {
			return chunk, nil
		}
		return append(r.buf, chunk...), nil
	}
}

func trimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\r' || b[0] == '\n') {
		b = b[1:]
	}
	for len(b) > 0 {
		c := b[len(b)-1]
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			break
		}
		b = b[:len(b)-1]
	}
	return b
}

func isDecodedType(t string) bool {
	switch RecordType(t) {
	case TypeAssistant, TypeUser, TypeAttachment, TypeSystem, TypeQueueOperation,
		TypeCustomTitle, TypeAITitle, TypeAgentName:
		return true
	}
	return false
}

// wireRecord mirrors a transcript line. Fields that vary in shape across versions stay json.RawMessage so one
// unexpected value can't cost us the whole record.
type wireRecord struct {
	Type        string `json:"type"`
	UUID        string `json:"uuid"`
	ParentUUID  string `json:"parentUuid"`
	RequestID   string `json:"requestId"`
	SessionID   string `json:"sessionId"`
	AgentID     string `json:"agentId"`
	Timestamp   string `json:"timestamp"`
	IsSidechain bool   `json:"isSidechain"`
	IsMeta      bool   `json:"isMeta"`
	CWD         string `json:"cwd"`
	GitBranch   string `json:"gitBranch"`
	Version     string `json:"version"`

	Message       *wireMessage    `json:"message"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`

	Attachment *struct {
		Type string `json:"type"`
	} `json:"attachment"`

	Subtype      string          `json:"subtype"`
	Content      json.RawMessage `json:"content"`
	DurationMs   float64         `json:"durationMs"`
	MessageCount int             `json:"messageCount"`
	Compact      *struct {
		Trigger    string  `json:"trigger"`
		PreTokens  int     `json:"preTokens"`
		PostTokens int     `json:"postTokens"`
		DurationMs float64 `json:"durationMs"`
	} `json:"compactMetadata"`

	Operation string `json:"operation"`

	CustomTitle string `json:"customTitle"`
	AITitle     string `json:"aiTitle"`
	AgentName   string `json:"agentName"`
}

type wireMessage struct {
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

type wireBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Signature string          `json:"signature"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

func (w wireRecord) toRecord(line, maxValue int) *Record {
	rec := &Record{
		Type:        RecordType(w.Type),
		Line:        line,
		UUID:        w.UUID,
		ParentUUID:  w.ParentUUID,
		RequestID:   w.RequestID,
		SessionID:   w.SessionID,
		AgentID:     w.AgentID,
		Timestamp:   parseTime(w.Timestamp),
		IsSidechain: w.IsSidechain,
		IsMeta:      w.IsMeta,
		CWD:         w.CWD,
		GitBranch:   w.GitBranch,
		Version:     w.Version,
	}

	if w.Message != nil {
		rec.Model = w.Message.Model
		if u := w.Message.Usage; u != nil {
			rec.Usage = &Usage{
				InputTokens:              u.InputTokens,
				OutputTokens:             u.OutputTokens,
				CacheCreationInputTokens: u.CacheCreationInputTokens,
				CacheReadInputTokens:     u.CacheReadInputTokens,
			}
		}
		rec.Prompt, rec.Blocks = decodeContent(w.Message.Content, maxValue)
	}

	rec.ToolUseResultBytes = len(w.ToolUseResult)
	if fits(len(w.ToolUseResult), maxValue) {
		rec.ToolUseResult = w.ToolUseResult
	}

	switch rec.Type {
	case TypeAttachment:
		if w.Attachment != nil {
			rec.Attachment = w.Attachment.Type
		}
	case TypeSystem:
		sys := &SystemInfo{
			Subtype:      w.Subtype,
			Content:      truncate(asString(w.Content), maxValue),
			Duration:     millis(w.DurationMs),
			MessageCount: w.MessageCount,
		}
		if c := w.Compact; c != nil {
			sys.Compact = &CompactInfo{
				Trigger:    c.Trigger,
				PreTokens:  c.PreTokens,
				PostTokens: c.PostTokens,
				Duration:   millis(c.DurationMs),
			}
		}
		rec.System = sys
	case TypeQueueOperation:
		rec.Queue = &QueueInfo{
			Operation: w.Operation,
			Content:   truncate(asString(w.Content), maxValue),
		}
	case TypeCustomTitle:
		rec.Title = w.CustomTitle
	case TypeAITitle:
		rec.Title = w.AITitle
	case TypeAgentName:
		rec.Title = w.AgentName
	}

	return rec
}

// decodeContent reads a message's content, which is a bare string when a person typed the message and an array of
// blocks otherwise.
func decodeContent(raw json.RawMessage, maxValue int) (prompt string, blocks []Block) {
	if len(raw) == 0 {
		return "", nil
	}
	if raw[0] == '"' {
		return truncate(asString(raw), maxValue), nil
	}

	var wires []wireBlock
	if err := json.Unmarshal(raw, &wires); err != nil {
		return "", nil
	}
	blocks = make([]Block, 0, len(wires))
	for _, w := range wires {
		blocks = append(blocks, w.toBlock(maxValue))
	}
	return "", blocks
}

func (w wireBlock) toBlock(maxValue int) Block {
	b := Block{
		Type:      BlockType(w.Type),
		Signature: w.Signature,
		IsError:   w.IsError,
		ToolName:  w.Name,
	}

	var text string
	switch b.Type {
	case BlockThinking:
		text = w.Thinking
	case BlockToolUse:
		b.ToolUseID = w.ID
		b.InputBytes = len(w.Input)
		b.Input, b.InputElided = decodeToolInput(w.Input, maxValue)
	case BlockToolResult:
		b.ToolUseID = w.ToolUseID
		text = flattenResultContent(w.Content)
	default:
		text = w.Text
	}

	b.TextBytes = len(text)
	b.Text = truncate(text, maxValue)
	b.Truncated = len(b.Text) < b.TextBytes
	return b
}

// decodeToolInput keeps a tool call's arguments one raw value at a time, dropping any single value larger than the cap.
// A `Write` call's `content` can run to megabytes while its `file_path` is 30 bytes, and the short keys are the ones
// that say what the call was doing.
func decodeToolInput(raw json.RawMessage, maxValue int) (map[string]json.RawMessage, []string) {
	if len(raw) == 0 {
		return nil, nil
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, nil
	}
	var elided []string
	for k, v := range all {
		if !fits(len(v), maxValue) {
			delete(all, k)
			elided = append(elided, k)
		}
	}
	sort.Strings(elided)
	return all, elided
}

// flattenResultContent turns a tool result's content into text. It's a string most of the time and an array of blocks
// the rest.
func flattenResultContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		return asString(raw)
	}
	var wires []wireBlock
	if err := json.Unmarshal(raw, &wires); err != nil {
		return ""
	}
	var parts []string
	for _, w := range wires {
		if BlockType(w.Type) == BlockText {
			parts = append(parts, w.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func asString(raw json.RawMessage) string {
	if len(raw) == 0 || raw[0] != '"' {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func fits(n, maxValue int) bool { return maxValue == Unlimited || n <= maxValue }

// truncate cuts s to maxValue bytes without splitting a rune.
func truncate(s string, maxValue int) string {
	if maxValue == Unlimited || len(s) <= maxValue {
		return s
	}
	cut := s[:maxValue]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

func millis(ms float64) time.Duration {
	return time.Duration(ms * float64(time.Millisecond))
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
