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

// thinkingInfo labels a thinking row with the model's own reasoning, on the rare transcript that kept it. Almost
// every thinking block arrives with its text stripped (5,469 of 5,471 sampled blocks), and labelThinking fills those
// in from what the agent did next.
func thinkingInfo(b transcript.Block) string { return clip(b.Text, subjectLimit) }

// labelThinking fills in the thinking rows whose block carried no text. The only honest label left is what the agent
// did next, so it's phrased as inference: "before Bash (checker): pnpm check". A reader can tell the two apart,
// because a borrowed label always starts with "before".
func labelThinking(rows []Row) {
	subject := "nothing followed it in the transcript"
	for i := len(rows) - 1; i >= 0; i-- {
		switch rows[i].Kind {
		case KindToolCall:
			subject = "before " + rows[i].Info
		case KindWriting:
			subject = "before writing: " + rows[i].Info
		case KindThinking:
			if rows[i].Info == "" {
				rows[i].Info = subject
			}
		}
	}
}

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
	if spawned := spawnInfo(c); spawned != "" {
		parts = append(parts, spawned)
	}
	if batch > 1 {
		parts = append(parts, "one of "+strconv.Itoa(batch)+" calls running at once")
	}
	return strings.Join(parts, "; ")
}

// compactionInfo says what a compaction achieved. A boundary that reports nothing still gets a row, because the
// moment is worth marking even when the numbers are missing.
func compactionInfo(c *transcript.CompactInfo) string {
	if c == nil || c.PreTokens == 0 {
		return "the context was compacted"
	}
	info := "compacted " + thousands(c.PreTokens) + " tokens down to " + thousands(c.PostTokens)
	if c.Trigger != "" {
		info += " (" + c.Trigger + ")"
	}
	return info
}

// thousands groups a count for reading: 674475 becomes 674,475.
func thousands(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + thousands(-n)
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}
