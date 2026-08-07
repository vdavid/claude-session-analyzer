// Package cache keeps a small summary of every session on disk, so a question about the whole corpus doesn't mean
// parsing 3.8 GB again.
//
// Two tiers, loaded lazily. `Digest` is a few KB and holds what a corpus query needs; `Detail` is tens of KB and holds
// the per-lane breakdown, read only when a query names lanes or agents. Rows are never cached: they'd be hundreds of
// megabytes, they're useless to a corpus query, and re-deriving them for one session is about a second. `docs/cache.md`
// has the reasoning and the layout.
package cache

import (
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/agg"
	"github.com/vdavid/claude-session-analyzer/internal/report"
	"github.com/vdavid/claude-session-analyzer/internal/session"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

// Version is the derivation this cache holds answers from. A digest is only valid for the rules that produced it, and a
// stale one is invisibly wrong, so bumping this is how a rule change invalidates the corpus.
//
// TestTheDigestVersionMovesWithTheDerivation ties it to two fingerprints, and either one moving fails that test with the
// number to write here:
//
//   - The golden CSV, which is what the derivation outputs for one fixture session.
//   - `timeline.ClassificationFingerprint`, which is the class and category mapping whatever that fixture holds. This
//     half exists because the golden alone missed a rule change twice: version 4 split `lint` out of `build`, and the
//     fixture's only compiler call is a `cargo build` that stayed a build, so every cached `cargo check` cell went stale
//     with the golden sitting still.
//
// The one reason to bump that neither fingerprint can see is a stored number or field changing meaning while the rules
// stay put. Version 3 added `Totals.NetSeconds` and took the stalls out of a tool group's `Seconds`; version 5 added
// `category` to every stored tool group, so a digest from version 4 carries groups with no category at all.
const Version = 5

// Digest is tier one: one session, summed, with no lane dimension. Every corpus query reads only these.
type Digest struct {
	// Version and Fingerprint are what make a digest usable. Fingerprint covers every file the session is written
	// across, so a subagent appending to its own transcript invalidates the session.
	Version     int    `json:"version"`
	Fingerprint string `json:"fingerprint"`
	// Zone is where the day boundaries were cut. A digest built in another zone answers "how much in July" differently,
	// so it counts as a miss.
	Zone string `json:"zone"`
	// Built is when this digest was written, for `cache info`.
	Built time.Time `json:"built"`

	Session report.Session `json:"session"`
	// Totals is the session's own summary. ByKind and ByTool are rolled up over every day, so they're the same numbers
	// the API reports.
	Totals report.Totals `json:"totals"`
	// Cells are the cube rolled up to kind, class, group, leaf, tool, and day, with the lane dimension dropped and its
	// distinct count kept. That's what a corpus `stats` query sums; it's a few KB because the lanes are gone.
	Cells []Cell `json:"cells"`
}

// Detail is tier two: the same session broken down by lane. Loaded only for a query that groups by or filters on a lane
// or an agent, which is rare enough that keeping it out of tier one is what holds the corpus scan to a few megabytes.
type Detail struct {
	Version     int    `json:"version"`
	Fingerprint string `json:"fingerprint"`
	Zone        string `json:"zone"`
	SessionID   string `json:"sessionId"`
	// Lanes describes each lane, so a query can report an agent's name and whether it was the lead.
	Lanes []DetailLane `json:"lanes"`
	// Cells keep the lane dimension, and are otherwise tier one's.
	Cells []Cell `json:"cells"`
}

type DetailLane struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	IsLead     bool    `json:"isLead"`
	WorkflowID string  `json:"workflowId,omitempty"`
	Model      string  `json:"model,omitempty"`
	Seconds    float64 `json:"seconds"`
	Rows       int     `json:"rows"`
}

// Cell is one cube cell on its way to disk. It's `agg.Cell` with seconds instead of a duration, because a duration
// marshals as an integer count of nanoseconds and this file is read by people too.
type Cell struct {
	Lane    string  `json:"lane,omitempty"`
	Agent   string  `json:"agent,omitempty"`
	Kind    string  `json:"kind,omitempty"`
	Class   string  `json:"class,omitempty"`
	Group   string  `json:"group,omitempty"`
	Leaf    string  `json:"leaf,omitempty"`
	Tool    string  `json:"tool,omitempty"`
	Day     string  `json:"day,omitempty"`
	Seconds float64 `json:"seconds"`
	Rows    int     `json:"rows,omitempty"`
	Calls   int     `json:"calls,omitempty"`
	// Errors, TimedOut, and Lanes are left out when they're zero, which is most cells.
	Errors   int `json:"errors,omitempty"`
	TimedOut int `json:"timedOut,omitempty"`
	Lanes    int `json:"lanes,omitempty"`
}

// Build sums a derived timeline into the two tiers.
func Build(sum session.Summary, tl *timeline.Timeline, fingerprint string, zone *time.Location, now time.Time) (Digest, Detail) {
	if zone == nil {
		zone = time.UTC
	}
	cube := agg.Build(tl, agg.Options{Zone: zone})
	cells := cube.Cells()

	totals := report.TotalsFrom(cells)
	totals.From = nilable(tl.First)
	totals.Until = nilable(tl.Last)
	totals.WallClockSeconds = report.Seconds(tl.Duration())
	totals.Lanes = len(tl.Lanes)

	digest := Digest{
		Version:     Version,
		Fingerprint: fingerprint,
		Zone:        zone.String(),
		Built:       now.UTC(),
		Session:     report.ForSession(sum),
		Totals:      totals,
		Cells:       toCells(agg.RollUp(cells, everythingButLane)),
	}

	detail := Detail{
		Version:     Version,
		Fingerprint: fingerprint,
		Zone:        zone.String(),
		SessionID:   sum.ID,
		Lanes:       make([]DetailLane, 0, len(tl.Lanes)),
		Cells:       toCells(agg.RollUp(cells, everythingButLane|agg.ByLane|agg.ByAgent)),
	}
	byLane := agg.RollUp(cells, agg.ByLane)
	for _, lane := range tl.Lanes {
		var rows int
		for _, c := range byLane {
			if c.Lane == lane.ID {
				rows = c.Rows
			}
		}
		detail.Lanes = append(detail.Lanes, DetailLane{
			ID:         lane.ID,
			Name:       lane.Name,
			IsLead:     lane.IsLead,
			WorkflowID: lane.WorkflowID,
			Model:      lane.Model,
			Seconds:    report.Seconds(lane.Duration()),
			Rows:       rows,
		})
	}
	return digest, detail
}

// everythingButLane is the grain both tiers are stored at. Tier one drops the lane and keeps its distinct count; tier
// two adds the lane back.
const everythingButLane = agg.ByKind | agg.ByClass | agg.ByGroup | agg.ByLeaf | agg.ByTool | agg.ByDay

// Cells turns stored cells back into cube cells, so a query rolls them up with the same code that built them.
func (d Digest) Cube() []agg.Cell { return fromCells(d.Cells) }

// Cube turns tier two's stored cells back into cube cells.
func (d Detail) Cube() []agg.Cell { return fromCells(d.Cells) }

func toCells(cells []agg.Cell) []Cell {
	out := make([]Cell, 0, len(cells))
	for _, c := range cells {
		out = append(out, Cell{
			Lane:     c.Lane,
			Agent:    c.Agent,
			Kind:     c.Kind,
			Class:    c.Class,
			Group:    c.Group,
			Leaf:     c.Leaf,
			Tool:     c.Tool,
			Day:      c.Day,
			Seconds:  report.Seconds(c.Duration),
			Rows:     c.Rows,
			Calls:    c.Calls,
			Errors:   c.Errors,
			TimedOut: c.TimedOut,
			Lanes:    c.Lanes,
		})
	}
	return out
}

func fromCells(cells []Cell) []agg.Cell {
	out := make([]agg.Cell, 0, len(cells))
	for _, c := range cells {
		out = append(out, agg.Cell{
			Key: agg.Key{
				Lane:  c.Lane,
				Agent: c.Agent,
				Kind:  c.Kind,
				Class: c.Class,
				Group: c.Group,
				Leaf:  c.Leaf,
				Tool:  c.Tool,
				Day:   c.Day,
			},
			Duration: time.Duration(c.Seconds * float64(time.Second)),
			Rows:     c.Rows,
			Calls:    c.Calls,
			Errors:   c.Errors,
			TimedOut: c.TimedOut,
			Lanes:    c.Lanes,
		})
	}
	return out
}

func nilable(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	utc := t.UTC()
	return &utc
}
