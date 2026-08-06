package timeline

import "time"

// Options tunes the derivation. The zero value is the one to use unless you're deliberately loosening a heuristic.
type Options struct {
	// CheapStall and HeavyStall are how long a tool result may take before the row is called a stall rather than a
	// tool execution. See stall.go for what separates cheap from heavy and why the numbers are where they are.
	CheapStall time.Duration
	HeavyStall time.Duration
	// MaxNamedTeammates caps how many teammates a lead's waiting row lists by name before it falls back to counting.
	// A session with a thousand workflow lanes would otherwise write a paragraph per row.
	MaxNamedTeammates int
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
	return o
}
