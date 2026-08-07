// Package agg rolls a derived timeline up into totals.
//
// One cube, many answers. The API's pie, the API's tool breakdown, a cached digest, and a `stats` query are all the
// same sum over the same rows, differing only in which dimensions they keep. Doing that once here means the rule that
// decides what counts as a call, or how a row crossing midnight is split, has one home rather than four.
package agg

import (
	"cmp"
	"slices"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

// Dim is one dimension of the cube, combined as a bitmask: `RollUp(ByGroup|ByDay)`.
type Dim uint16

const (
	ByLane Dim = 1 << iota
	ByAgent
	ByKind
	ByClass
	ByGroup
	ByLeaf
	ByTool
	ByDay
)

// Key is what a cell is grouped by. A field the roll-up dropped is empty, so a cell's key says exactly which question
// it answers.
type Key struct {
	// Lane is the lane id, which is what to group by. Two lanes carry the same name when neither has a `.meta.json`.
	Lane  string
	Agent string
	Kind  string
	// Class, Group, Leaf, and Tool are empty on a row that isn't about a tool. Tool is the raw name the harness used,
	// which a reader greps a transcript for; Group and Leaf are the two names a breakdown shows.
	Class string
	Group string
	Leaf  string
	Tool  string
	// Day is the local date the time was spent on, `2006-01-02`, in the zone Options named.
	Day string
}

// Cell is one group's totals.
type Cell struct {
	Key
	// Duration is wall clock added up. Rows that ran at the same time each count their own, the way lane time does.
	Duration time.Duration
	// Rows counts activity rows. Calls counts only the row a tool call actually ran in, so it's the number of times a
	// tool was used rather than twice that.
	Rows     int
	Calls    int
	Errors   int
	TimedOut int
	// Lanes is how many distinct lanes contributed. It can't be added up from a finer roll-up: one lane calling two of
	// a server's methods is one lane for the server and one for each method.
	Lanes int
}

// Options configures the build.
type Options struct {
	// Zone is where the day boundaries are. A session runs for days and the question is nearly always "how much in
	// July", so the cut has to be local midnight rather than UTC. Defaults to UTC.
	Zone *time.Location
}

// Cube is a session's rows, pre-summed at the finest grain any caller asks for.
type Cube struct {
	cells map[Key]*Cell
}

// Build sums a timeline into a cube.
func Build(tl *timeline.Timeline, opts Options) *Cube {
	zone := opts.Zone
	if zone == nil {
		zone = time.UTC
	}

	c := &Cube{cells: make(map[Key]*Cell)}
	for _, r := range tl.Rows {
		key := Key{
			Lane:  r.LaneID,
			Agent: r.Agent,
			Kind:  string(r.Kind),
			Class: string(r.Class),
			Group: r.ToolGroup,
			Leaf:  r.ToolLeaf,
			Tool:  r.Tool,
		}
		// The counts land on the day the row started and the seconds are spread over the days the row covered, so
		// rolling the day dimension away gives the row back whole rather than counting it once per midnight crossed.
		for i, part := range splitByDay(r.From, r.Until, zone) {
			key.Day = part.day
			cell := c.cell(key)
			cell.Duration += part.d
			if i > 0 {
				continue
			}
			cell.Rows++
			if r.IsToolRun() {
				cell.Calls++
				if r.IsError {
					cell.Errors++
				}
				if r.TimedOut {
					cell.TimedOut++
				}
			}
		}
	}
	return c
}

func (c *Cube) cell(key Key) *Cell {
	cell, ok := c.cells[key]
	if !ok {
		cell = &Cell{Key: key}
		c.cells[key] = cell
	}
	return cell
}

// RollUp sums the cube down to the dimensions asked for, keeping every other key field empty. `RollUp(0)` is the whole
// session in one cell. The result is sorted by key, so two runs over the same session agree.
func RollUp(cells []Cell, dims Dim) []Cell {
	type acc struct {
		cell  Cell
		lanes map[string]bool
	}
	into := make(map[Key]*acc, len(cells))

	for _, in := range cells {
		key := mask(in.Key, dims)
		a, ok := into[key]
		if !ok {
			a = &acc{cell: Cell{Key: key}, lanes: map[string]bool{}}
			into[key] = a
		}
		a.cell.Duration += in.Duration
		a.cell.Rows += in.Rows
		a.cell.Calls += in.Calls
		a.cell.Errors += in.Errors
		a.cell.TimedOut += in.TimedOut
		// A cell that arrived already rolled up past the lane dimension carries a count instead of a lane to add to the
		// set. Both can't happen for one cell, so taking the larger is exact rather than a guess.
		if in.Lane != "" {
			a.lanes[in.Lane] = true
		} else if in.Lanes > a.cell.Lanes {
			a.cell.Lanes = in.Lanes
		}
	}

	out := make([]Cell, 0, len(into))
	for _, a := range into {
		if len(a.lanes) > 0 {
			a.cell.Lanes = len(a.lanes)
		}
		out = append(out, a.cell)
	}
	slices.SortFunc(out, byKey)
	return out
}

// RollUp sums the whole cube down to the dimensions asked for.
func (c *Cube) RollUp(dims Dim) []Cell {
	return RollUp(c.Cells(), dims)
}

// Cells is every cell at the finest grain, for a caller that wants to filter before rolling up.
func (c *Cube) Cells() []Cell {
	out := make([]Cell, 0, len(c.cells))
	for _, cell := range c.cells {
		out = append(out, *cell)
	}
	slices.SortFunc(out, byKey)
	return out
}

// A tool call spends its time on three clocks, and every one of them arrives on a cell carrying the group's name. Ask
// the predicates below rather than testing the kind, and ask them before rolling the kind dimension away, because
// that's where the evidence is.

// IsComposing says the cell holds the agent's clock writing the call. It's most of what an `Edit` costs, because the
// model streams the whole diff as the call's arguments.
func (c Cell) IsComposing() bool { return c.IsAboutATool() && c.Kind == string(timeline.KindToolCall) }

// IsStall says a stall verdict landed on the cell: the call came back far too late for what it was doing
// (`timeline/stall.go`). The agent was suspended rather than working, so leaving this inside a tool's own clock makes
// one suspension read as the tool being slow.
func (c Cell) IsStall() bool { return c.IsAboutATool() && c.Kind == string(timeline.KindStalled) }

// IsToolRun says the cell holds the stretch a call actually ran for, whatever verdict the derivation reached, the stall
// included. That's why it's the one that counts calls.
func (c Cell) IsToolRun() bool { return c.IsAboutATool() && !c.IsComposing() }

// IsAboutATool says the cell carries a tool's name, whichever of the three clocks it holds. A question about tools has
// nothing to ask of the rest, so it leaves them out entirely rather than folding them into a nameless group.
func (c Cell) IsAboutATool() bool { return c.Group != "" }

// ToolRuns keeps only the cells holding a tool's own clock, and it's what anything reporting on tools has to filter
// through first.
//
// Every call leaves two rows: the agent composing it, and the tool running. Both carry the tool's name, so a total that
// takes them all reports a tool as costing more than it did, and the difference is invisible in the output. Stalls come
// along, because a stall was a call and counting calls is what this is for; Stalls is how a caller takes their time out
// again.
func ToolRuns(cells []Cell) []Cell { return keep(cells, Cell.IsToolRun) }

// Composing keeps only the cells holding the agent's own clock while it wrote a tool call, which is the complement of
// ToolRuns over the cells carrying a group's name.
func Composing(cells []Cell) []Cell { return keep(cells, Cell.IsComposing) }

// Stalls keeps only the cells a stall verdict landed on. Their time is inside what ToolRuns returns, so a breakdown
// reporting what a tool cost subtracts these and reports them beside it.
func Stalls(cells []Cell) []Cell { return keep(cells, Cell.IsStall) }

func keep(cells []Cell, wanted func(Cell) bool) []Cell {
	out := make([]Cell, 0, len(cells))
	for _, c := range cells {
		if wanted(c) {
			out = append(out, c)
		}
	}
	return out
}

// Sum adds up a set of cells without keeping any dimension, which is the denominator most questions need.
func Sum(cells []Cell) Cell {
	rolled := RollUp(cells, 0)
	if len(rolled) == 0 {
		return Cell{}
	}
	return rolled[0]
}

func byKey(a, b Cell) int {
	return cmp.Or(
		cmp.Compare(a.Day, b.Day),
		cmp.Compare(a.Lane, b.Lane),
		cmp.Compare(a.Agent, b.Agent),
		cmp.Compare(a.Kind, b.Kind),
		cmp.Compare(a.Class, b.Class),
		cmp.Compare(a.Group, b.Group),
		cmp.Compare(a.Leaf, b.Leaf),
		cmp.Compare(a.Tool, b.Tool),
	)
}

// mask blanks every key field the roll-up isn't keeping.
func mask(k Key, dims Dim) Key {
	out := Key{}
	if dims&ByLane != 0 {
		out.Lane = k.Lane
	}
	if dims&ByAgent != 0 {
		out.Agent = k.Agent
	}
	if dims&ByKind != 0 {
		out.Kind = k.Kind
	}
	if dims&ByClass != 0 {
		out.Class = k.Class
	}
	if dims&ByGroup != 0 {
		out.Group = k.Group
	}
	if dims&ByLeaf != 0 {
		out.Leaf = k.Leaf
	}
	if dims&ByTool != 0 {
		out.Tool = k.Tool
	}
	if dims&ByDay != 0 {
		out.Day = k.Day
	}
	return out
}

// dayPart is how much of a row fell on one local date.
type dayPart struct {
	day string
	d   time.Duration
}

const dayLayout = "2006-01-02"

// splitByDay cuts a row's span at local midnight. A row that stays inside one day is one part, which is nearly all of
// them; an overnight `pnpm check` or a lead waiting for a person overnight is several.
func splitByDay(from, until time.Time, zone *time.Location) []dayPart {
	start := from.In(zone)
	if !until.After(from) {
		// Zero-length rows are ordinary here: a tool call composed in the same record as the block before it has no
		// span at all, and it still has to be counted somewhere.
		return []dayPart{{day: start.Format(dayLayout), d: 0}}
	}

	var parts []dayPart
	end := until.In(zone)
	for cursor := start; cursor.Before(end); {
		midnight := time.Date(cursor.Year(), cursor.Month(), cursor.Day(), 0, 0, 0, 0, zone).AddDate(0, 0, 1)
		stop := midnight
		if stop.After(end) {
			stop = end
		}
		parts = append(parts, dayPart{day: cursor.Format(dayLayout), d: stop.Sub(cursor)})
		cursor = stop
	}
	return parts
}
