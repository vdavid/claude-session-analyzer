package transcript

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// readAll drains a reader over the named fixture and returns everything it produced.
func readAll(t *testing.T, path string, opts Options) ([]*Record, Stats) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	r := NewReader(f, opts)
	var got []*Record
	for r.Next() {
		got = append(got, r.Record())
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return got, r.Stats()
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return ts
}

func TestReaderDecodesEveryRecordType(t *testing.T) {
	got, _ := readAll(t, filepath.Join("testdata", "lane.jsonl"), Options{})

	// One line holds a type this package doesn't decode, so the fixture's 14 lines yield 13 records.
	if len(got) != 13 {
		t.Fatalf("record count = %d, want 13", len(got))
	}

	tests := []struct {
		name  string
		index int
		check func(t *testing.T, rec *Record)
	}{
		{
			name:  "a custom title is decoded so a session list can show it",
			index: 0,
			check: func(t *testing.T, rec *Record) {
				if rec.Type != TypeCustomTitle {
					t.Errorf("type = %q, want %q", rec.Type, TypeCustomTitle)
				}
				if rec.Title != "Trying a thing" {
					t.Errorf("title = %q", rec.Title)
				}
				if !rec.Timestamp.IsZero() {
					t.Errorf("timestamp = %v, want zero: title records carry none", rec.Timestamp)
				}
			},
		},
		{
			name:  "user prompt carries its text and context",
			index: 1,
			check: func(t *testing.T, rec *Record) {
				if rec.Type != TypeUser {
					t.Errorf("type = %q, want %q", rec.Type, TypeUser)
				}
				if rec.Prompt != "Please count the widgets." {
					t.Errorf("prompt = %q", rec.Prompt)
				}
				if len(rec.Blocks) != 0 {
					t.Errorf("blocks = %d, want 0 for a string prompt", len(rec.Blocks))
				}
				if rec.CWD != "/tmp/alpha" || rec.GitBranch != "main" || rec.Version != "2.1.220" {
					t.Errorf("context = %q %q %q", rec.CWD, rec.GitBranch, rec.Version)
				}
				if want := mustTime(t, "2026-08-03T08:44:00Z"); !rec.Timestamp.Equal(want) {
					t.Errorf("timestamp = %v, want %v", rec.Timestamp, want)
				}
				if rec.Line != 2 {
					t.Errorf("line = %d, want 2", rec.Line)
				}
			},
		},
		{
			name:  "thinking block keeps its signature and empty text",
			index: 2,
			check: func(t *testing.T, rec *Record) {
				if rec.Type != TypeAssistant {
					t.Fatalf("type = %q", rec.Type)
				}
				if rec.Model != "claude-opus-5" || rec.RequestID != "req_1" {
					t.Errorf("model = %q, requestId = %q", rec.Model, rec.RequestID)
				}
				if len(rec.Blocks) != 1 || rec.Blocks[0].Type != BlockThinking {
					t.Fatalf("blocks = %+v", rec.Blocks)
				}
				if rec.Blocks[0].Text != "" || rec.Blocks[0].Signature != "sig-abc" {
					t.Errorf("thinking = %q, signature = %q", rec.Blocks[0].Text, rec.Blocks[0].Signature)
				}
				if rec.Usage == nil {
					t.Fatal("usage is nil")
				}
				if rec.Usage.OutputTokens != 120 || rec.Usage.CacheReadInputTokens != 4000 {
					t.Errorf("usage = %+v", rec.Usage)
				}
			},
		},
		{
			name:  "tool use keeps its id, name, and input",
			index: 3,
			check: func(t *testing.T, rec *Record) {
				if len(rec.Blocks) != 1 {
					t.Fatalf("blocks = %d", len(rec.Blocks))
				}
				b := rec.Blocks[0]
				if b.Type != BlockToolUse || b.ToolUseID != "toolu_1" || b.ToolName != "Bash" {
					t.Fatalf("block = %+v", b)
				}
				var cmd string
				if err := json.Unmarshal(b.Input["command"], &cmd); err != nil {
					t.Fatalf("decode command: %v", err)
				}
				if cmd != "ls -la /tmp/alpha" {
					t.Errorf("command = %q", cmd)
				}
			},
		},
		{
			name:  "tool result pairs on the tool use id",
			index: 4,
			check: func(t *testing.T, rec *Record) {
				if rec.Type != TypeUser || len(rec.Blocks) != 1 {
					t.Fatalf("record = %+v", rec)
				}
				b := rec.Blocks[0]
				if b.Type != BlockToolResult || b.ToolUseID != "toolu_1" {
					t.Fatalf("block = %+v", b)
				}
				if !strings.HasPrefix(b.Text, "total 0") {
					t.Errorf("text = %q", b.Text)
				}
				if b.IsError {
					t.Error("is_error should be false")
				}
				if len(rec.ToolUseResult) == 0 {
					t.Error("toolUseResult was dropped")
				}
			},
		},
		{
			name:  "attachment keeps its kind",
			index: 5,
			check: func(t *testing.T, rec *Record) {
				if rec.Type != TypeAttachment || rec.Attachment != "hook_success" {
					t.Errorf("record = %q / %q", rec.Type, rec.Attachment)
				}
			},
		},
		{
			name:  "text block carries the prose",
			index: 6,
			check: func(t *testing.T, rec *Record) {
				if len(rec.Blocks) != 1 || rec.Blocks[0].Type != BlockText {
					t.Fatalf("blocks = %+v", rec.Blocks)
				}
				if rec.Blocks[0].Text != "There are three widgets." {
					t.Errorf("text = %q", rec.Blocks[0].Text)
				}
			},
		},
		{
			name:  "queue enqueue carries what the user typed and when",
			index: 7,
			check: func(t *testing.T, rec *Record) {
				if rec.Type != TypeQueueOperation {
					t.Fatalf("type = %q", rec.Type)
				}
				if rec.Queue == nil {
					t.Fatal("queue info is nil")
				}
				if rec.Queue.Operation != "enqueue" || rec.Queue.Content != "Also count the sprockets." {
					t.Errorf("queue = %+v", rec.Queue)
				}
				if want := mustTime(t, "2026-08-03T08:44:13Z"); !rec.Timestamp.Equal(want) {
					t.Errorf("timestamp = %v, want %v", rec.Timestamp, want)
				}
			},
		},
		{
			name:  "one record can hold several blocks including parallel tool calls",
			index: 8,
			check: func(t *testing.T, rec *Record) {
				if len(rec.Blocks) != 4 {
					t.Fatalf("blocks = %d, want 4", len(rec.Blocks))
				}
				want := []BlockType{BlockThinking, BlockText, BlockToolUse, BlockToolUse}
				for i, w := range want {
					if rec.Blocks[i].Type != w {
						t.Errorf("block %d = %q, want %q", i, rec.Blocks[i].Type, w)
					}
				}
				if rec.Blocks[0].Text != "Two calls can run at once." {
					t.Errorf("thinking text = %q, want the reasoning kept when it's present", rec.Blocks[0].Text)
				}
				if rec.Blocks[2].ToolUseID != "toolu_2" || rec.Blocks[3].ToolUseID != "toolu_3" {
					t.Errorf("tool use ids = %q, %q", rec.Blocks[2].ToolUseID, rec.Blocks[3].ToolUseID)
				}
			},
		},
		{
			name:  "tool result with array content is flattened and its error flag kept",
			index: 9,
			check: func(t *testing.T, rec *Record) {
				if len(rec.Blocks) != 1 {
					t.Fatalf("blocks = %d", len(rec.Blocks))
				}
				b := rec.Blocks[0]
				if !b.IsError {
					t.Error("is_error should be true")
				}
				if b.Text != "No such file\nor directory" {
					t.Errorf("text = %q", b.Text)
				}
			},
		},
		{
			name:  "harness-injected user records are marked, not hidden",
			index: 10,
			check: func(t *testing.T, rec *Record) {
				if !rec.IsMeta {
					t.Error("isMeta should be true")
				}
				if len(rec.Blocks) != 1 || rec.Blocks[0].Type != BlockText {
					t.Fatalf("blocks = %+v", rec.Blocks)
				}
			},
		},
		{
			name:  "turn duration is decoded",
			index: 11,
			check: func(t *testing.T, rec *Record) {
				if rec.Type != TypeSystem || rec.System == nil {
					t.Fatalf("record = %+v", rec)
				}
				if rec.System.Subtype != "turn_duration" {
					t.Errorf("subtype = %q", rec.System.Subtype)
				}
				if rec.System.Duration != 25*time.Second || rec.System.MessageCount != 12 {
					t.Errorf("duration = %v, messages = %d", rec.System.Duration, rec.System.MessageCount)
				}
			},
		},
		{
			name:  "compaction reports how long it took and what it dropped",
			index: 12,
			check: func(t *testing.T, rec *Record) {
				if rec.System == nil || rec.System.Compact == nil {
					t.Fatalf("record = %+v", rec)
				}
				c := rec.System.Compact
				if c.Trigger != "manual" || c.PreTokens != 674475 || c.PostTokens != 10198 {
					t.Errorf("compact = %+v", c)
				}
				if c.Duration != 132016*time.Millisecond {
					t.Errorf("compact duration = %v", c.Duration)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, got[tt.index])
		})
	}
}

func TestReaderSkipsUnknownTypesWithoutFailing(t *testing.T) {
	_, stats := readAll(t, filepath.Join("testdata", "lane.jsonl"), Options{})

	if stats.Lines != 14 {
		t.Errorf("lines = %d, want 14", stats.Lines)
	}
	if stats.Decoded != 13 {
		t.Errorf("decoded = %d, want 13", stats.Decoded)
	}
	want := map[string]int{"future-record-type": 1}
	if len(stats.SkippedTypes) != len(want) {
		t.Fatalf("skipped types = %v, want %v", stats.SkippedTypes, want)
	}
	for k, v := range want {
		if stats.SkippedTypes[k] != v {
			t.Errorf("skipped[%q] = %d, want %d", k, stats.SkippedTypes[k], v)
		}
	}
	if stats.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", stats.Skipped)
	}
	if stats.Decoded+stats.Skipped+stats.Blank+stats.Malformed != stats.Lines {
		t.Errorf("stats don't account for every line: %+v", stats)
	}
	if stats.Malformed != 0 {
		t.Errorf("malformed = %d, want 0", stats.Malformed)
	}
}

func TestReaderSurvivesLinesFarBeyondTheScannerDefault(t *testing.T) {
	// Real transcripts hold lines over 1 MB, well past `bufio.Scanner`'s 64 KB default.
	const payload = 3 << 20
	path := filepath.Join(t.TempDir(), "huge.jsonl")
	line, err := json.Marshal(map[string]any{
		"type":      "assistant",
		"uuid":      "big",
		"timestamp": "2026-08-03T08:44:05.000Z",
		"message": map[string]any{
			"role":    "assistant",
			"model":   "claude-opus-5",
			"content": []any{map[string]any{"type": "text", "text": strings.Repeat("x", payload)}},
		},
	})
	if err != nil {
		t.Fatalf("build line: %v", err)
	}
	body := "{\"type\":\"mode\",\"mode\":\"normal\"}\n" + string(line) + "\n{\"type\":\"mode\",\"mode\":\"normal\"}\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, stats := readAll(t, path, Options{MaxValueBytes: Unlimited})

	if len(got) != 1 {
		t.Fatalf("records = %d, want 1", len(got))
	}
	if len(got[0].Blocks) != 1 || len(got[0].Blocks[0].Text) != payload {
		t.Errorf("text length = %d, want %d", len(got[0].Blocks[0].Text), payload)
	}
	if stats.LongestLine < len(line) {
		t.Errorf("longest line = %d, want at least %d", stats.LongestLine, len(line))
	}
	if stats.Malformed != 0 {
		t.Errorf("malformed = %d, want 0", stats.Malformed)
	}
}

func TestReaderCountsMalformedLinesAndKeepsGoing(t *testing.T) {
	got, stats := readAll(t, filepath.Join("testdata", "malformed.jsonl"), Options{})

	if len(got) != 2 {
		t.Fatalf("records = %d, want the two decodable ones", len(got))
	}
	if got[0].UUID != "ok1" || got[1].UUID != "ok2" {
		t.Errorf("uuids = %q, %q", got[0].UUID, got[1].UUID)
	}
	if stats.Malformed != 1 {
		t.Errorf("malformed = %d, want 1", stats.Malformed)
	}
	if stats.FirstMalformedLine != 2 {
		t.Errorf("first malformed line = %d, want 2", stats.FirstMalformedLine)
	}
	if stats.FirstMalformedErr == nil {
		t.Error("first malformed error should be recorded")
	}
	if stats.Blank != 1 {
		t.Errorf("blank = %d, want 1", stats.Blank)
	}
}

func TestReaderCapsLargeValuesSoWholeCorpusFitsInMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fat.jsonl")
	fat := strings.Repeat("y", 5000)
	body := `{"type":"assistant","uuid":"a","timestamp":"2026-08-03T08:44:05.000Z","message":{"role":"assistant","content":[` +
		`{"type":"tool_use","id":"t1","name":"Write","input":{"file_path":"/tmp/a.txt","content":"` + fat + `"}}]}}` + "\n" +
		`{"type":"user","uuid":"u","timestamp":"2026-08-03T08:44:06.000Z","message":{"role":"user","content":[` +
		`{"type":"tool_result","tool_use_id":"t1","content":"` + fat + `"}]},"toolUseResult":{"stdout":"` + fat + `"}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, _ := readAll(t, path, Options{MaxValueBytes: 1000})

	use := got[0].Blocks[0]
	if _, ok := use.Input["file_path"]; !ok {
		t.Error("a small input key must survive so tool classification still works")
	}
	if _, ok := use.Input["content"]; ok {
		t.Error("an oversized input key must be dropped, not truncated into invalid JSON")
	}
	if len(use.InputElided) != 1 || use.InputElided[0] != "content" {
		t.Errorf("elided keys = %v, want [content]", use.InputElided)
	}

	result := got[1].Blocks[0]
	if len(result.Text) != 1000 {
		t.Errorf("result text length = %d, want it truncated to 1000", len(result.Text))
	}
	if !result.Truncated || result.TextBytes != 5000 {
		t.Errorf("truncated = %v, original size = %d, want true and 5000", result.Truncated, result.TextBytes)
	}
	if got[1].ToolUseResult != nil {
		t.Error("an oversized toolUseResult must be dropped whole, so what's kept is always valid JSON")
	}
	if got[1].ToolUseResultBytes < 5000 {
		t.Errorf("toolUseResult size = %d, want the real size recorded", got[1].ToolUseResultBytes)
	}
}

func TestReaderReportsEmptyInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, stats := readAll(t, path, Options{})

	if len(got) != 0 || stats.Lines != 0 || stats.Decoded != 0 {
		t.Errorf("records = %d, stats = %+v", len(got), stats)
	}
}

func TestReaderHandlesLineEndingEdgeCases(t *testing.T) {
	record := `{"type":"assistant","uuid":"%s","timestamp":"2026-08-03T08:44:05.000Z",` +
		`"message":{"role":"assistant","content":[{"type":"text","text":"Fine."}]}}`

	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "a final line with no trailing newline still counts",
			body: fmt.Sprintf(record, "a") + "\n" + fmt.Sprintf(record, "b"),
			want: []string{"a", "b"},
		},
		{
			name: "carriage returns are trimmed",
			body: fmt.Sprintf(record, "a") + "\r\n" + fmt.Sprintf(record, "b") + "\r\n",
			want: []string{"a", "b"},
		},
		{
			name: "blank lines in the middle are stepped over",
			body: fmt.Sprintf(record, "a") + "\n\n   \n" + fmt.Sprintf(record, "b") + "\n",
			want: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "lane.jsonl")
			if err := os.WriteFile(path, []byte(tt.body), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			got, stats := readAll(t, path, Options{})

			if len(got) != len(tt.want) {
				t.Fatalf("records = %d, want %d", len(got), len(tt.want))
			}
			for i, w := range tt.want {
				if got[i].UUID != w {
					t.Errorf("record %d = %q, want %q", i, got[i].UUID, w)
				}
			}
			if stats.Malformed != 0 {
				t.Errorf("malformed = %d, want 0", stats.Malformed)
			}
		})
	}
}

func TestReaderStopsAtAReadError(t *testing.T) {
	r := NewReader(&errAfter{body: []byte(`{"type":"assistant","uuid":"a"}` + "\n")}, Options{})

	var got int
	for r.Next() {
		got++
	}

	if got != 1 {
		t.Errorf("records = %d, want the one that came before the error", got)
	}
	if r.Err() == nil {
		t.Error("a read error should surface through Err, unlike a malformed line")
	}
}

// errAfter hands out its body once and then fails, the way a truncated read does.
type errAfter struct {
	body []byte
	done bool
}

func (e *errAfter) Read(p []byte) (int, error) {
	if e.done {
		return 0, errors.New("disk went away")
	}
	e.done = true
	return copy(p, e.body), nil
}
