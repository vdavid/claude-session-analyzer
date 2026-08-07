// Package stats answers one shape of question over a set of sessions: keep the cells a filter names, group what's
// left, and add it up.
//
// One grammar rather than one function per question. "How much of my agents' time went into the checker script", "do
// they reach for codegraph or for grep", and "what's the net time, with the waiting taken out" are the same sum over
// the same cube, differing only in which cells they keep and which dimensions they keep them by.
package stats

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/vdavid/claude-session-analyzer/internal/agg"
	"github.com/vdavid/claude-session-analyzer/internal/report"
)

// Source is one session's contribution to a query: its cube cells, plus the few things a session-level dimension needs
// that no cell carries.
type Source struct {
	SessionID   string
	ProjectName string
	ProjectSlug string
	Title       string
	// Cells are the session's cube at whatever grain the caller loaded. Cells summed without the lane dimension answer
	// everything except a lane or an agent question, so the caller gets to choose how much it reads.
	Cells []agg.Cell
	// WallClockSeconds is the session's first record to its last.
	WallClockSeconds float64
	// Lanes is how many lanes the session had, the lead included.
	Lanes int
}

// project is what the project dimension groups by: the name where there is one, and the slug on disk otherwise, so a
// session whose transcript never recorded a working directory still lands somewhere rather than in a nameless bucket
// with every other such session.
func (s Source) project() string {
	if s.ProjectName != "" {
		return s.ProjectName
	}
	return s.ProjectSlug
}

// Result is one answer, with the denominators next to the number so a share can be checked.
type Result struct {
	Scope   Scope   `json:"scope"`
	Totals  Totals  `json:"totals"`
	Matched Matched `json:"matched"`
	// Groups are the matched cells rolled up to Spec.GroupBy, biggest first. Empty when nothing was grouped by, in
	// which case Matched is the whole answer.
	Groups []Group `json:"groups"`
	// Truncated is how many groups Spec.Top left out, zero when nothing was cut.
	Truncated int `json:"truncated,omitempty"`
	// Notes are what a reader has to know to read the answer right.
	Notes []string `json:"notes,omitempty"`
}

// Scope says what the answer covers, so a stored result can be read without the query beside it. The session count is
// in Totals.
type Scope struct {
	Projects int `json:"projects"`
	// FirstDay and LastDay bracket the days the cells cover, as `2006-01-02` in the zone the cube was built in. Empty
	// when no cell carries a day.
	FirstDay string `json:"firstDay,omitempty"`
	LastDay  string `json:"lastDay,omitempty"`
	// GroupBy and Where echo the question.
	GroupBy []Dim    `json:"groupBy,omitempty"`
	Where   []Clause `json:"where,omitempty"`
}

// Totals covers every session in scope, unfiltered. These are the denominators.
type Totals struct {
	// WallClockSeconds is added up across sessions, so two sessions that ran side by side count their overlap twice.
	// It's the honest denominator for one session and a loose one for a corpus.
	WallClockSeconds float64 `json:"wallClockSeconds"`
	// LaneTimeSeconds is every lane's rows added up, which is larger than wall clock whenever lanes ran at once.
	// ActiveSeconds is lane time with every gap taken out: what the agents spent working rather than waiting.
	LaneTimeSeconds float64 `json:"laneTimeSeconds"`
	ActiveSeconds   float64 `json:"activeSeconds"`
	Rows            int     `json:"rows"`
	Lanes           int     `json:"lanes"`
	Sessions        int     `json:"sessions"`
}

// Measures are what a query counts, for the whole match and for each group.
type Measures struct {
	Seconds float64 `json:"seconds"`
	Rows    int     `json:"rows"`
	// Calls counts the times a tool ran, which is one per call rather than two. See Run.
	Calls int `json:"calls"`
	// Lanes is how many lanes contributed, counted per session and added up across them, because a lane belongs to one
	// session.
	Lanes    int `json:"lanes"`
	Errors   int `json:"errors"`
	TimedOut int `json:"timedOut"`
}

// Matched is what the Where clauses kept, with its share of each of the three denominators. A share is a fraction, not
// a percentage, and it's zero when the denominator is.
type Matched struct {
	Measures
	ShareOfLaneTime  float64 `json:"shareOfLaneTime"`
	ShareOfActive    float64 `json:"shareOfActive"`
	ShareOfWallClock float64 `json:"shareOfWallClock"`
}

// Key is what a group is keyed by. Only the dimensions the query grouped by are filled in; the rest are empty.
type Key struct {
	Kind    string `json:"kind,omitempty"`
	Class   string `json:"class,omitempty"`
	Group   string `json:"group,omitempty"`
	Leaf    string `json:"leaf,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Day     string `json:"day,omitempty"`
	Lane    string `json:"lane,omitempty"`
	Agent   string `json:"agent,omitempty"`
	Session string `json:"session,omitempty"`
	Project string `json:"project,omitempty"`
}

// Group is one row of the answer.
type Group struct {
	Key
	Measures
	ShareOfLaneTime float64 `json:"shareOfLaneTime"`
}

// Value reads one dimension off a key, so a caller can print a column per grouped dimension without a switch of its
// own. An ungrouped dimension is empty.
func (k Key) Value(dim Dim) string {
	switch dim {
	case DimKind:
		return k.Kind
	case DimClass:
		return k.Class
	case DimGroup:
		return k.Group
	case DimLeaf:
		return k.Leaf
	case DimTool:
		return k.Tool
	case DimDay:
		return k.Day
	case DimLane:
		return k.Lane
	case DimAgent:
		return k.Agent
	case DimSession:
		return k.Session
	case DimProject:
		return k.Project
	}
	return ""
}

func (k *Key) set(dim Dim, value string) {
	switch dim {
	case DimKind:
		k.Kind = value
	case DimClass:
		k.Class = value
	case DimGroup:
		k.Group = value
	case DimLeaf:
		k.Leaf = value
	case DimTool:
		k.Tool = value
	case DimDay:
		k.Day = value
	case DimLane:
		k.Lane = value
	case DimAgent:
		k.Agent = value
	case DimSession:
		k.Session = value
	case DimProject:
		k.Project = value
	}
}

// Run answers the question.
//
// A query that names a tool dimension counts only the rows a tool actually ran in (`agg.ToolRuns`). Every call leaves
// two rows, the agent composing it and the tool running, and both carry the tool's name, so a total that takes them
// all reports the checker as costing more than it did and the answer looks perfectly reasonable. The cost of the rule
// is that such a query has no `tool call` kind left in it, which is correct: that row is the agent's, not the tool's.
// `Spec.IncludeComposingRows` turns it off for the rare question about the agent instead.
//
// The denominators in Totals are always the unfiltered whole, composing rows included, so a share says what part of
// the session went somewhere rather than what part of the filter did.
func Run(spec Spec, sessions []Source) (Result, error) {
	if err := spec.validate(); err != nil {
		return Result{}, err
	}

	result := Result{
		Scope:  scopeOf(spec, sessions),
		Totals: totalsOf(sessions),
	}

	toolRunsOnly := spec.aboutTools() && !spec.IncludeComposingRows
	matched := &acc{}
	groups := map[Key]*acc{}

	for _, source := range sessions {
		cells := source.Cells
		if toolRunsOnly {
			cells = agg.ToolRuns(cells)
		}
		for _, cell := range cells {
			if !keeps(spec, cell, source) {
				continue
			}
			matched.add(cell, source.SessionID)
			if len(spec.GroupBy) == 0 {
				continue
			}
			key := keyOf(cell, source, spec.GroupBy)
			into, ok := groups[key]
			if !ok {
				into = &acc{}
				groups[key] = into
			}
			into.add(cell, source.SessionID)
		}
	}

	result.Matched = Matched{
		Measures:         matched.measures(),
		ShareOfLaneTime:  share(matched.seconds(), result.Totals.LaneTimeSeconds),
		ShareOfActive:    share(matched.seconds(), result.Totals.ActiveSeconds),
		ShareOfWallClock: share(matched.seconds(), result.Totals.WallClockSeconds),
	}

	result.Groups = make([]Group, 0, len(groups))
	for key, into := range groups {
		result.Groups = append(result.Groups, Group{
			Key:             key,
			Measures:        into.measures(),
			ShareOfLaneTime: share(into.seconds(), result.Totals.LaneTimeSeconds),
		})
	}
	sortGroups(result.Groups, spec.GroupBy)
	if spec.Top > 0 && len(result.Groups) > spec.Top {
		result.Truncated = len(result.Groups) - spec.Top
		result.Groups = result.Groups[:spec.Top]
	}

	result.Notes = notesFor(spec, sessions, toolRunsOnly)
	return result, nil
}

// keeps says the cell satisfies every clause: ANDed across clauses, ORed inside one.
func keeps(spec Spec, cell agg.Cell, source Source) bool {
	for _, clause := range spec.Where {
		if !clause.matches(valueOf(clause.Field, cell, source)) {
			return false
		}
	}
	return true
}

// valueOf reads one dimension off a cell. The last two come from the session rather than the cell, because a cube
// holds one session and has nothing to say about which.
func valueOf(dim Dim, cell agg.Cell, source Source) string {
	switch dim {
	case DimKind:
		return cell.Kind
	case DimClass:
		return cell.Class
	case DimGroup:
		return cell.Group
	case DimLeaf:
		return cell.Leaf
	case DimTool:
		return cell.Tool
	case DimDay:
		return cell.Day
	case DimLane:
		return cell.Lane
	case DimAgent:
		return cell.Agent
	case DimSession:
		return source.SessionID
	case DimProject:
		return source.project()
	}
	return ""
}

func keyOf(cell agg.Cell, source Source, dims []Dim) Key {
	var key Key
	for _, dim := range dims {
		key.set(dim, valueOf(dim, cell, source))
	}
	return key
}

// acc adds cells up.
type acc struct {
	duration time.Duration
	rows     int
	calls    int
	errors   int
	timedOut int
	// lanes and counted are both per session, because a lane belongs to one: two sessions with three lanes each are
	// six lanes, not three. Cells that carry a lane land in the set; cells summed past the lane dimension carry a count
	// instead, and the largest count seen is the best that evidence supports.
	lanes   map[string]map[string]bool
	counted map[string]int
}

func (a *acc) add(cell agg.Cell, session string) {
	a.duration += cell.Duration
	a.rows += cell.Rows
	a.calls += cell.Calls
	a.errors += cell.Errors
	a.timedOut += cell.TimedOut

	switch {
	case cell.Lane != "":
		if a.lanes == nil {
			a.lanes = map[string]map[string]bool{}
		}
		if a.lanes[session] == nil {
			a.lanes[session] = map[string]bool{}
		}
		a.lanes[session][cell.Lane] = true
	case cell.Lanes > a.counted[session]:
		if a.counted == nil {
			a.counted = map[string]int{}
		}
		a.counted[session] = cell.Lanes
	}
}

func (a *acc) seconds() float64 { return report.Seconds(a.duration) }

func (a *acc) measures() Measures {
	lanes := 0
	for _, set := range a.lanes {
		lanes += len(set)
	}
	for session, count := range a.counted {
		// A session whose cells named their lanes has already been counted exactly, so its count is the weaker
		// evidence and doesn't get to add to it.
		if len(a.lanes[session]) == 0 {
			lanes += count
		}
	}
	return Measures{
		Seconds:  a.seconds(),
		Rows:     a.rows,
		Calls:    a.calls,
		Lanes:    lanes,
		Errors:   a.errors,
		TimedOut: a.timedOut,
	}
}

// totalsOf sums every session in scope, unfiltered. Lane time and active seconds come from `report.TotalsFrom`, so the
// denominator here is the same number the API reports for the same sessions.
func totalsOf(sessions []Source) Totals {
	var (
		cells []agg.Cell
		wall  float64
		lanes int
	)
	for _, source := range sessions {
		cells = append(cells, source.Cells...)
		wall += source.WallClockSeconds
		lanes += source.Lanes
	}

	whole := report.TotalsFrom(cells)
	return Totals{
		WallClockSeconds: roundSeconds(wall),
		LaneTimeSeconds:  whole.LaneTimeSeconds,
		ActiveSeconds:    whole.ActiveSeconds,
		Rows:             whole.Rows,
		Lanes:            lanes,
		Sessions:         len(sessions),
	}
}

func scopeOf(spec Spec, sessions []Source) Scope {
	scope := Scope{GroupBy: spec.GroupBy, Where: spec.Where}
	projects := map[string]bool{}
	for _, source := range sessions {
		projects[source.project()] = true
		for _, cell := range source.Cells {
			if cell.Day == "" {
				continue
			}
			if scope.FirstDay == "" || cell.Day < scope.FirstDay {
				scope.FirstDay = cell.Day
			}
			if cell.Day > scope.LastDay {
				scope.LastDay = cell.Day
			}
		}
	}
	scope.Projects = len(projects)
	return scope
}

// notesFor says what a reader has to know to read the answer right. A note is for something the numbers can't show on
// their own, so an ordinary answer carries none.
func notesFor(spec Spec, sessions []Source, toolRunsOnly bool) []string {
	if len(sessions) == 0 {
		return []string{"No sessions are in scope, so every number here is zero. Widen the range, or check the session or project the query named."}
	}

	var notes []string
	for _, dim := range []Dim{DimLane, DimAgent} {
		if !spec.names(dim) || carries(sessions, dim) {
			continue
		}
		notes = append(notes, fmt.Sprintf(
			"These sessions were summed with the lanes rolled away, so nothing here can be told apart by %s. Load the per-lane detail for them and ask again.", dim))
		break // Both dimensions travel together, and one note says it.
	}

	switch {
	case toolRunsOnly:
		notes = append(notes, "Counting only the rows a tool ran in. The `tool call` row beside each one is the agent composing the call, and counting both would report every tool as costing about twice what it did, so a breakdown by kind shows no `tool call` here.")
	case spec.aboutTools():
		notes = append(notes, "Composing rows are included, so each call counts twice: once for the agent writing it, once for the tool running it. That's a question about the agent rather than about the tool.")
	}
	return notes
}

// carries says at least one cell in scope has something to say about a dimension. Lane and agent are the two that can
// be missing wholesale, because a session's own summary is stored with them rolled away.
func carries(sessions []Source, dim Dim) bool {
	for _, source := range sessions {
		for _, cell := range source.Cells {
			if valueOf(dim, cell, source) != "" {
				return true
			}
		}
	}
	return false
}

// sortGroups puts the biggest first, breaks a tie on calls, and falls back to the key in the order the caller grouped
// by, so two runs over the same sessions agree.
func sortGroups(groups []Group, dims []Dim) {
	slices.SortFunc(groups, func(a, b Group) int {
		if c := cmp.Or(cmp.Compare(b.Seconds, a.Seconds), cmp.Compare(b.Calls, a.Calls)); c != 0 {
			return c
		}
		for _, dim := range dims {
			if c := cmp.Compare(a.Value(dim), b.Value(dim)); c != 0 {
				return c
			}
		}
		return 0
	})
}

// share is a fraction of a denominator, and zero when there's nothing to divide by. A session with no timestamped
// record has a wall clock of zero, and 99 of the 725 sessions on the machine this was built against are that.
func share(part, whole float64) float64 {
	if whole <= 0 {
		return 0
	}
	return math.Round(part/whole*1e6) / 1e6
}

// roundSeconds rounds a sum of already-rounded seconds back to the millisecond `report.Seconds` works in, so adding
// hundreds of sessions up doesn't leave float noise in the answer.
func roundSeconds(seconds float64) float64 { return math.Round(seconds*1000) / 1000 }
