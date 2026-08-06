package timeline

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// subjectLimit is how much of a command, a path, or a sentence the Info column carries. Long enough to recognise the
// call, short enough that a spreadsheet stays readable.
const subjectLimit = 110

// subjectKeys are the tool input fields that say what a call was about, most telling first. Reading them in order
// means one rule covers every tool, including MCP tools nobody has seen yet.
var subjectKeys = []string{"command", "file_path", "path", "pattern", "query", "url", "prompt", "description"}

// subjectOf pulls the short phrase that names what a call was doing.
func subjectOf(b transcript.Block) string {
	for _, key := range subjectKeys {
		if s := inputString(b, key); s != "" {
			return clip(s, subjectLimit)
		}
	}
	for _, key := range subjectKeys {
		if elided(b, key) {
			return "(" + key + " too large to keep)"
		}
	}
	return ""
}

// inputString reads one tool input field as text. Fields that aren't strings (a timeout, a line count) come back
// empty, which is what a subject wants.
func inputString(b transcript.Block, key string) string {
	raw, ok := b.Input[key]
	if !ok || len(raw) == 0 || raw[0] != '"' {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// inputInt reads one tool input field as a whole number, which is how a Bash timeout arrives.
func inputInt(b transcript.Block, key string) (int, bool) {
	raw, ok := b.Input[key]
	if !ok || len(raw) == 0 {
		return 0, false
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, false
	}
	return int(n), true
}

func elided(b transcript.Block, key string) bool {
	for _, k := range b.InputElided {
		if k == key {
			return true
		}
	}
	return false
}

// callInfo labels a tool call row: the tool, what kind of work it is, and what it was about.
func callInfo(c *call) string {
	return describe(c.block.ToolName, c.class, c.subject)
}

func describe(tool string, class ToolClass, subject string) string {
	head := tool
	if class != "" && class != ClassOther {
		head += " (" + string(class) + ")"
	}
	if subject == "" {
		return head
	}
	return head + ": " + subject
}

// clip shortens text to a single readable line of at most n characters, collapsing the whitespace a heredoc or a
// multi-line command carries.
func clip(s string, n int) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	out := b.String()
	if len(out) <= n {
		return out
	}
	cut := out[:n]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1] // never cut a rune in half
	}
	return cut + "..."
}

// thinkingInfo labels a thinking row.
func thinkingInfo(b transcript.Block) string { return "" }

// executionInfo labels a tool execution or a stalled row: what ran, and anything unusual about how it ended.
func executionInfo(c *call, v verdict, batch int) string {
	parts := []string{describe(c.block.ToolName, c.class, c.subject)}

	switch {
	case v.kind == KindStalled:
		parts = append(parts, "no result for "+FormatDuration(c.end.Sub(c.start))+
			", past the "+FormatDuration(v.threshold)+
			" a "+string(c.class)+" call can plausibly take")
	case !c.resolved:
		parts = append(parts, "no result in the transcript, the tool may still have been running")
	case v.timedOut:
		parts = append(parts, "timed out after "+FormatDuration(v.timeout))
	}
	if c.result.IsError {
		parts = append(parts, "the tool reported a failure")
	}
	if batch > 1 {
		parts = append(parts, "one of "+strconv.Itoa(batch)+" calls running at once")
	}
	return strings.Join(parts, "; ")
}
