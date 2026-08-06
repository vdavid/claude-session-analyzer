package timeline

import "time"

// DefaultMaxResponseSpan is how long a single model response may plausibly take, including API queue time on a large
// context, before the derivation reads the stretch as idle time rather than as the model working.
//
// It's a backstop, not the main rule: a turn ending and input arriving are the evidence the derivation prefers, and
// this only catches the lanes that carry neither. A resumed session is the case that needs it, where the only thing
// between `/exit` and 25 days later is a text block, and calling that 596 hours of writing would be absurd.
//
// Fifteen minutes sits in the gap between two populations. Measured across every session on this machine with the
// backstop off (verified 2026-08-06), thinking spans fall away sharply through the real range, 2,880 rows past one
// minute, 520 past two, 60 past five, and then stop falling: 31 past ten minutes, 16 past thirty, 12 past an hour.
// That flat tail is idle time, not responses. Tool call spans make the same point more bluntly, with 717 rows past a
// minute and not one past twenty.
const DefaultMaxResponseSpan = 15 * time.Minute

// Options tunes the derivation. The zero value is the one to use unless you're deliberately loosening a heuristic.
type Options struct {
	// CheapStall and HeavyStall are how long a tool result may take before the row is called a stall rather than a
	// tool execution. See stall.go for what separates cheap from heavy and why the numbers are where they are.
	CheapStall time.Duration
	HeavyStall time.Duration
	// MaxNamedTeammates caps how many teammates a lead's waiting row lists by name before it falls back to counting.
	// A session with a thousand workflow lanes would otherwise write a paragraph per row.
	MaxNamedTeammates int
	// MaxResponseSpan is how long one model response may plausibly take, queue time included, before the stretch is
	// read as the lane sitting idle instead. See DefaultMaxResponseSpan.
	MaxResponseSpan time.Duration
}

func (o Options) withDefaults() Options {
	if o.CheapStall == 0 {
		o.CheapStall = DefaultCheapStall
	}
	if o.HeavyStall == 0 {
		o.HeavyStall = DefaultHeavyStall
	}
	if o.MaxNamedTeammates == 0 {
		o.MaxNamedTeammates = 5
	}
	if o.MaxResponseSpan == 0 {
		o.MaxResponseSpan = DefaultMaxResponseSpan
	}
	return o
}
