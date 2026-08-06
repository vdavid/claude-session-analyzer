package timeline

import (
	"encoding/json"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/transcript"
)

// DefaultCheapStall and DefaultHeavyStall are how long a tool may take before its row stops being a tool execution and
// becomes a stall. Getting this wrong in either direction hurts: too low and the tool calls an honest 40-minute build
// a stall, too high and it reports a six-hour suspension as a six-hour `rm`.
//
// So the threshold depends on what the call was doing, and both numbers sit far above anything their class does in
// practice:
//
//   - Cheap classes have no plausible long form: removing a file, reading one, listing a directory, grepping a tree, a
//     git command, an MCP round trip. An hour of that isn't the tool working, even over a slow network mount. The
//     stall that prompted this tool was a 6h15m `rm` plus `du -sh`, six times over the line.
//   - Heavy classes earn their time: a build, a test suite, a checker script, a dev server the agent left running, a
//     wait loop that exists to block, a teammate the agent is blocked on. Twelve hours is past anything honest and
//     still catches a suspension that ran overnight.
//
// Two calls are never stalls whatever they cost. One that timed out was ended on purpose by the harness, and one whose
// result never arrived has no end to measure: we closed it at the moment we stopped reading.
//
// This stays a heuristic, so a stalled row says how long it waited and what the call was, and the raw duration is
// right there to be judged.
const (
	DefaultCheapStall = time.Hour
	DefaultHeavyStall = 12 * time.Hour
)

// BashTimeoutCap is the longest the harness lets a Bash call run, whatever timeout the call asks for. It's the
// `BASH_MAX_TIMEOUT_MS` default, and a deployment can raise it, so it's a ceiling on inference rather than a fact
// about a given call. Verified 2026-08-06 against the reference session, where four calls asking for 1,200s to 3,600s
// all came back at 600.1s.
const BashTimeoutCap = 10 * time.Minute

// timeoutSlack is how far past its limit a call may come back and still count as having hit it. Every timeout in the
// reference session landed within a second of its limit; the one call that ran 171s past its limit came back with real
// output, so the timeout isn't what ended it.
const timeoutSlack = time.Minute

// stallThreshold is how long this class may run before its row is called a stall.
func stallThreshold(class ToolClass, opts Options) time.Duration {
	switch class {
	case ClassBuild, ClassTest, ClassChecker, ClassDevServer, ClassWait, ClassAgent:
		return opts.HeavyStall
	default:
		return opts.CheapStall
	}
}

// verdict is what the derivation concluded about one finished call.
type verdict struct {
	kind     Kind
	timedOut bool
	// timeout is the limit the call hit, when it hit one.
	timeout time.Duration
	// threshold is the stall line this call was measured against, so the row can say what it was.
	threshold time.Duration
}

// judge decides whether a call's duration is honest work, a timeout, or a stall.
func judge(c *call, opts Options) verdict {
	v := verdict{kind: KindToolExecution, threshold: stallThreshold(c.class, opts)}
	if c.class == ClassAsk {
		// The call is open because a person hasn't answered yet. That's idle time, not a tool running and not a
		// suspended agent: across every session on this machine, three of the five rows the stall rule flagged were
		// questions left open for hours.
		v.kind = KindWaitingForPerson
		return v
	}
	if !c.resolved {
		return v
	}

	took := c.end.Sub(c.start)
	if limit, ok := timedOut(c, took); ok {
		v.timedOut, v.timeout = true, limit
		return v
	}
	if took >= v.threshold {
		v.kind = KindStalled
	}
	return v
}

// timedOut says the harness cut the call short, and how long it let it run.
//
// The harness records the limit it enforced in `timedOutAfterMs`, which settles it. When it doesn't, the only evidence
// left is the call's own requested limit: a call that ran for as long as it was allowed and came back within a minute
// of that hit its timeout. In the reference session that second rule doubles what the first one finds.
//
// A call that asked for no limit is left alone. The harness default is two minutes, so inferring from it would put an
// ordinary two-minute command right on the line and call it a timeout it never hit.
func timedOut(c *call, took time.Duration) (time.Duration, bool) {
	if ms, ok := payloadInt(c.record, "timedOutAfterMs"); ok {
		return time.Duration(ms) * time.Millisecond, true
	}
	requested, ok := inputInt(c.block, "timeout")
	if !ok || requested <= 0 {
		return 0, false
	}
	limit := time.Duration(requested) * time.Millisecond
	if c.block.ToolName == "Bash" && limit > BashTimeoutCap {
		limit = BashTimeoutCap
	}
	if took >= limit-time.Second && took <= limit+timeoutSlack {
		return limit, true
	}
	return 0, false
}

// payloadString reads a text field off a tool result's structured payload.
func payloadString(rec *transcript.Record, key string) (string, bool) {
	raw, ok := payloadField(rec, key)
	if !ok || len(raw) == 0 || raw[0] != '"' {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, s != ""
}

// payloadInt reads a whole number off a tool result's structured payload. The payload is a bare string as often as
// it's an object, and both are normal, so a shape that doesn't fit isn't an error.
func payloadInt(rec *transcript.Record, key string) (int, bool) {
	raw, ok := payloadField(rec, key)
	if !ok {
		return 0, false
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, false
	}
	return int(n), true
}

// payloadField pulls one field out of a tool result's structured payload. The payload is a bare string as often as
// it's an object, and both are normal, so a shape that doesn't fit isn't an error.
func payloadField(rec *transcript.Record, key string) (json.RawMessage, bool) {
	if rec == nil || len(rec.ToolUseResult) == 0 || rec.ToolUseResult[0] != '{' {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rec.ToolUseResult, &fields); err != nil {
		return nil, false
	}
	raw, ok := fields[key]
	return raw, ok
}
