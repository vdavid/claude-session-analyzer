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
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
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
	// ToolClocksApart says each call's three clocks were kept apart, because the query is about tools: Measures.Seconds
	// is the tool running, with ComposingSeconds and StalledSeconds beside it rather than inside it. False on any other
	// question, where Seconds is every matched cell's clock and the other two are subsets of it, because a breakdown by
	// kind or by day has to partition lane time. A renderer labels its time column from this.
	ToolClocksApart bool `json:"toolClocksApart"`
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
	// LaneTimeSeconds, NetSeconds, and ActiveSeconds are the ladder: every lane's clock added up, then minus the waits
	// whose clock belongs to a person or a teammate, then minus the gaps net keeps. Each is the one above minus
	// something, so they're never rivals. The arithmetic and the reason each rung exists: `docs/api.md`.
	LaneTimeSeconds float64 `json:"laneTimeSeconds"`
	NetSeconds      float64 `json:"netSeconds"`
	ActiveSeconds   float64 `json:"activeSeconds"`
	Rows            int     `json:"rows"`
	Lanes           int     `json:"lanes"`
	Sessions        int     `json:"sessions"`
}

// Measures are what a query counts, for the whole match and for each group.
type Measures struct {
	// Seconds is the matched cells' clock. On a question about tools (Result.ToolClocksApart) the two measures below
	// are carved out of it and it means the tool running; on any other question it's every matched cell, because a
	// breakdown by kind or by day has to partition lane time.
	Seconds float64 `json:"seconds"`
	// ComposingSeconds is the agent's clock writing the calls, and StalledSeconds is the calls that came back far too
	// late to have been running. Both are per-call clocks that would otherwise read as the tool's own, and they invert
	// per tool: `Edit` is nearly all composing, `Bash (checker)` nearly all running. A stall is still one of Calls,
	// because it was a call; a composing row never was one.
	ComposingSeconds float64 `json:"composingSeconds"`
	StalledSeconds   float64 `json:"stalledSeconds"`
	Rows             int     `json:"rows"`
	// Calls counts the times a tool ran, which is one per call rather than two. See Run.
	Calls int `json:"calls"`
	// Lanes is how many lanes contributed, counted per session and added up across them, because a lane belongs to one
	// session.
	Lanes int `json:"lanes"`
	// Sessions is how many sessions contributed, counted distinctly. Like Lanes it doesn't add up: a session that shows
	// up in two groups counts once in each and once in the match, so a group's count is never the sum of a finer
	// roll-up's. Against Totals.Sessions it answers "codegraph was used in 12 of 735 sessions".
	Sessions int `json:"sessions"`
	Errors   int `json:"errors"`
	TimedOut int `json:"timedOut"`
}

// Matched is what the Where clauses kept, with its share of each denominator: the ladder's three rungs and wall clock.
// A share is a fraction, not a percentage, it's over Seconds alone, and it's zero when the denominator is.
type Matched struct {
	Measures
	ShareOfLaneTime  float64 `json:"shareOfLaneTime"`
	ShareOfNet       float64 `json:"shareOfNet"`
	ShareOfActive    float64 `json:"shareOfActive"`
	ShareOfWallClock float64 `json:"shareOfWallClock"`
}

// Key is what a group is keyed by. Only the dimensions the query grouped by are filled in; the rest are empty.
type Key struct {
	Kind string `json:"kind,omitempty"`
	// Category is the coarse bucket a tool call falls in, derived from the class and the group by `internal/timeline`
	// rather than stored on a cell, so a taxonomy change needs no new cache shape.
	Category string `json:"category,omitempty"`
	Class    string `json:"class,omitempty"`
	Group    string `json:"group,omitempty"`
	Leaf     string `json:"leaf,omitempty"`
	Tool     string `json:"tool,omitempty"`
	Day      string `json:"day,omitempty"`
	Lane     string `json:"lane,omitempty"`
	Agent    string `json:"agent,omitempty"`
	Session  string `json:"session,omitempty"`
	Project  string `json:"project,omitempty"`
}

// Group is one row of the answer, ordered by weight.
type Group struct {
	Key
	Measures
	// ShareOfLaneTime is Seconds over lane time, so on a question about tools it's the share of lane time the tool spent
	// running rather than the share the group accounts for: the other two clocks stay named beside it instead of being
	// folded into one percentage. Groups are ordered by weight rather than by this, so a group carrying one long stall
	// sits high with a small share, which is the anomaly showing itself rather than a sorting bug.
	ShareOfLaneTime float64 `json:"shareOfLaneTime"`
}

// Value reads one dimension off a key, so a caller can print a column per grouped dimension without a switch of its
// own. An ungrouped dimension is empty.
func (k Key) Value(dim Dim) string {
	switch dim {
	case DimKind:
		return k.Kind
	case DimCategory:
		return k.Category
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
	case DimCategory:
		k.Category = value
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
// A query that names a tool dimension keeps each call's three clocks apart, because all three arrive on a cell carrying
// the tool's name and adding them together reports the checker as costing what the agent and a suspended session cost,
// while the answer looks perfectly reasonable. `Measures.Seconds` is the tool running, `ComposingSeconds` is the agent
// writing the call, and `StalledSeconds` is a call that came back far too late to have been running. The cost of the
// rule is that such a query has no `tool call` kind left in it, which is correct: that row is the agent's, not the
// tool's, and its time is in `composingSeconds` rather than dropped. `Spec.IncludeComposingRows` turns the whole split
// off for the rare question about the agent instead.
//
// The denominators in Totals are always the unfiltered whole, composing rows included, so a share says what part of
// the session went somewhere rather than what part of the filter did.
func Run(spec Spec, sessions []Source) (Result, error) {
	if err := spec.validate(); err != nil {
		return Result{}, err
	}

	result := Result{
		Scope:           scopeOf(spec, sessions),
		Totals:          totalsOf(sessions),
		ToolClocksApart: spec.aboutTools() && !spec.IncludeComposingRows,
	}

	matched := &acc{}
	groups := map[Key]*acc{}

	for _, source := range sessions {
		for _, cell := range source.Cells {
			// A question about tools has nothing to ask of a cell carrying no tool, and folding those into a group
			// keyed by an empty name would put the session's thinking and waiting under a nameless slice.
			if result.ToolClocksApart && !cell.IsAboutATool() {
				continue
			}
			if !keeps(spec, cell, source) {
				continue
			}
			matched.add(cell, source.SessionID, result.ToolClocksApart)
			if len(spec.GroupBy) == 0 {
				continue
			}
			key := keyOf(cell, source, spec.GroupBy)
			into, ok := groups[key]
			if !ok {
				into = &acc{}
				groups[key] = into
			}
			into.add(cell, source.SessionID, result.ToolClocksApart)
		}
	}

	result.Matched = Matched{
		Measures:         matched.measures(),
		ShareOfLaneTime:  share(matched.seconds(), result.Totals.LaneTimeSeconds),
		ShareOfNet:       share(matched.seconds(), result.Totals.NetSeconds),
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
	sortGroups(result.Groups, spec.GroupBy, result.ToolClocksApart)
	if spec.Top > 0 && len(result.Groups) > spec.Top {
		result.Truncated = len(result.Groups) - spec.Top
		result.Groups = result.Groups[:spec.Top]
	}

	result.Notes = notesFor(spec, sessions, result.ToolClocksApart)
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
	case DimCategory:
		// Derived rather than read off the cell: a category is a pure function of the class and the group, both of which
		// every cell already carries, so the taxonomy can move without a new stored field.
		return string(timeline.CategoryOf(timeline.ToolClass(cell.Class), cell.Group))
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
	// composing and stalled are the two clocks of a call that aren't the tool running. They're always summed, and
	// whether they're also inside duration is what `apart` decides. See add.
	composing time.Duration
	stalled   time.Duration
	rows      int
	calls     int
	errors    int
	timedOut  int
	// lanes and counted are both per session, because a lane belongs to one: two sessions with three lanes each are
	// six lanes, not three. Cells that carry a lane land in the set; cells summed past the lane dimension carry a count
	// instead, and the largest count seen is the best that evidence supports.
	lanes   map[string]map[string]bool
	counted map[string]int
	// sessions is the distinct set that contributed, keyed the same way, so a session showing up in two groups counts
	// once in each rather than twice in either.
	sessions map[string]bool
}

// add folds one cell in. `apart` is Result.ToolClocksApart: with it, a call's composing and stalled time comes out of
// duration so what's left is the tool running, and the composing row stops counting as a row because it never was a
// call. Without it the cell is added whole, because a breakdown by kind or by day has to partition lane time, and the
// two clocks are reported as the subsets of it they are.
func (a *acc) add(cell agg.Cell, session string, apart bool) {
	composing, stalled := cell.IsComposing(), cell.IsStall()
	switch {
	case composing:
		a.composing += cell.Duration
	case stalled:
		a.stalled += cell.Duration
	}

	if !apart || !composing {
		a.rows += cell.Rows
		a.calls += cell.Calls
		a.errors += cell.Errors
		a.timedOut += cell.TimedOut
	}
	if !apart || !(composing || stalled) {
		a.duration += cell.Duration
	}

	if a.sessions == nil {
		a.sessions = map[string]bool{}
	}
	a.sessions[session] = true

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
		Seconds:          a.seconds(),
		ComposingSeconds: report.Seconds(a.composing),
		StalledSeconds:   report.Seconds(a.stalled),
		Rows:             a.rows,
		Calls:            a.calls,
		Lanes:            lanes,
		Sessions:         len(a.sessions),
		Errors:           a.errors,
		TimedOut:         a.timedOut,
	}
}

// totalsOf sums every session in scope, unfiltered. The ladder comes from `report.TotalsFrom`, so the denominators here
// are the same numbers the API reports for the same sessions.
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
		NetSeconds:       whole.NetSeconds,
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
func notesFor(spec Spec, sessions []Source, clocksApart bool) []string {
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
	case clocksApart:
		notes = append(notes, "A call's time is split three ways, because all three parts arrive under the tool's name: the tool running (`seconds`), the agent composing the call (`composingSeconds`), and a call that came back far too late to have been running (`stalledSeconds`). Adding them together reports a tool as costing what the agent and a suspended session cost, so read the one you mean. Grouped by kind, the `tool call` row reports no running time at all: its clock is the composing one.")
	case spec.aboutTools():
		notes = append(notes, "Composing rows are added into `seconds` here, so each call counts twice: once for the agent writing it, once for the tool running it. That's a question about the agent rather than about the tool.")
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

// weight is what "biggest first" means for one group, and what Spec.Top keeps.
//
// On a question about tools it's every clock the group holds, not only the tool's own: a group whose time went into
// composing its calls (`Edit` is 2.24 h composing against 0.03 h running on the reference session) or into one stall
// (`Bash (file write)` is 6.26 h of one suspended `rm`) would otherwise sink below groups costing a fraction of it, and
// `--top 8` would hide exactly the rows worth looking at. Anywhere else the other two clocks are subsets of Seconds, so
// adding them would count time twice.
func (g Group) weight(clocksApart bool) float64 {
	if !clocksApart {
		return g.Seconds
	}
	return g.Seconds + g.ComposingSeconds + g.StalledSeconds
}

// sortGroups puts the biggest first, breaks a tie on calls, and falls back to the key in the order the caller grouped
// by, so two runs over the same sessions agree.
func sortGroups(groups []Group, dims []Dim, clocksApart bool) {
	slices.SortFunc(groups, func(a, b Group) int {
		if c := cmp.Or(
			cmp.Compare(b.weight(clocksApart), a.weight(clocksApart)),
			cmp.Compare(b.Calls, a.Calls),
		); c != 0 {
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
