package stats

import (
	"fmt"
	"strings"

	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

// Dim is one dimension of a query. The same names are the `--group-by` values and the `--where` fields, so a caller
// learns one vocabulary and uses it in both places.
type Dim string

const (
	DimKind    Dim = "kind"
	DimClass   Dim = "class"
	DimGroup   Dim = "group"
	DimLeaf    Dim = "leaf"
	DimTool    Dim = "tool"
	DimDay     Dim = "day"
	DimLane    Dim = "lane"
	DimAgent   Dim = "agent"
	DimSession Dim = "session"
	DimProject Dim = "project"
)

// Dims lists every dimension, in the order a message and the vocabulary show them: what the agent was doing, what it
// was doing it with, and where it happened.
var Dims = []Dim{
	DimKind, DimClass, DimGroup, DimLeaf, DimTool, DimDay, DimLane, DimAgent, DimSession, DimProject,
}

// toolDims are the dimensions that only say something about a row a tool was involved in. Naming one of them makes a
// query a question about tools, which is what turns the composing rows off. See Run.
var toolDims = map[Dim]bool{DimClass: true, DimGroup: true, DimLeaf: true, DimTool: true}

// laneDims are the two dimensions a session's own summary can't answer, because it was summed with the lanes rolled
// away. A query naming one of them against such cells gets a note rather than a narrower answer.
var laneDims = map[Dim]bool{DimLane: true, DimAgent: true}

// classes are the tool classes the engine can label a call with, in the order `internal/timeline/tool.go` declares
// them. The engine exports no list, so this one is a copy; TestTheClassListMatchesTheEngines reads that file and fails
// with the name to add here when a class is added there.
var classes = []string{
	"checker", "build", "lint", "test", "dev server", "wait", "git", "search", "file read", "file write",
	"agent", "ask", "mcp", "web", "shell", "other",
}

// Spec is one question.
type Spec struct {
	// Where is ANDed across clauses and ORed inside one, so `--where class=checker,test --where day=2026-08-03` reads
	// as "a checker or a test run, on that day".
	Where []Clause
	// GroupBy is in the order the caller asked for, which is the order two groups with the same seconds are compared
	// in. Empty means the answer is Result.Matched and there are no groups.
	GroupBy []Dim
	// Top keeps only the biggest groups, and Result.Truncated says how many it left out. Zero keeps everything.
	Top int
	// IncludeComposingRows turns off the tool-run filter Run applies to a question about tools. See Run.
	IncludeComposingRows bool
}

// Clause is one filter: a field, and the values that satisfy it.
type Clause struct {
	Field Dim `json:"field"`
	// Values are ORed. Matching ignores case, a leading or trailing `*` globs, and everything else has to match
	// exactly.
	Values []string `json:"values"`
}

// Vocab is everything a query can name. An agent guessing whether a class is `checker` or `checker-script` guesses
// wrong, so the CLI prints this rather than sending anyone to the source.
type Vocab struct {
	Dims    []Dim    `json:"dimensions"`
	Kinds   []string `json:"kinds"`
	Classes []string `json:"classes"`
}

// Vocabulary is what a caller can say: the dimensions, the activity kinds the engine derives, and the classes it puts
// tool calls into.
func Vocabulary() Vocab {
	kinds := make([]string, 0, len(timeline.Kinds))
	for _, kind := range timeline.Kinds {
		kinds = append(kinds, string(kind))
	}
	return Vocab{
		Dims:    append([]Dim(nil), Dims...),
		Kinds:   kinds,
		Classes: append([]string(nil), classes...),
	}
}

// ParseSpec builds a Spec from the strings a CLI has on its flags: a repeated `--where field=value[,value]`, one
// `--group-by a,b`, and a `--top`. An empty group-by is a question with one answer rather than a mistake.
func ParseSpec(where []string, groupBy string, top int) (Spec, error) {
	if top < 0 {
		return Spec{}, fmt.Errorf("a top of %d would keep nothing. Pass a positive number of groups to keep, or leave it out to keep all of them", top)
	}

	spec := Spec{Top: top}
	for _, raw := range where {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		clause, err := ParseClause(raw)
		if err != nil {
			return Spec{}, err
		}
		spec.Where = append(spec.Where, clause)
	}

	dims, err := ParseGroupBy(groupBy)
	if err != nil {
		return Spec{}, err
	}
	spec.GroupBy = dims
	return spec, nil
}

// ParseGroupBy reads a comma-separated list of dimensions, keeping the caller's order and dropping a repeat: grouping
// by the same thing twice is grouping by it once.
func ParseGroupBy(s string) ([]Dim, error) {
	var out []Dim
	seen := map[Dim]bool{}
	for _, name := range splitList(s) {
		dim, err := ParseDim(name)
		if err != nil {
			return nil, err
		}
		if seen[dim] {
			continue
		}
		seen[dim] = true
		out = append(out, dim)
	}
	return out, nil
}

// ParseClause reads one `field=value[,value]` filter.
func ParseClause(s string) (Clause, error) {
	field, rest, found := strings.Cut(s, "=")
	if !found {
		return Clause{}, fmt.Errorf("a filter reads as field=value, and %q has no = in it. Try something like class=checker, with the field one of %s", s, dimList())
	}
	dim, err := ParseDim(field)
	if err != nil {
		return Clause{}, err
	}
	values := splitList(rest)
	if len(values) == 0 {
		return Clause{}, fmt.Errorf("the filter %q has nothing to match on. Put one or more values after the =, separated by commas, as in %s=something", s, dim)
	}
	return Clause{Field: dim, Values: values}, nil
}

// ParseDim reads one dimension name.
func ParseDim(s string) (Dim, error) {
	name := Dim(strings.ToLower(strings.TrimSpace(s)))
	for _, dim := range Dims {
		if dim == name {
			return dim, nil
		}
	}
	return "", fmt.Errorf("%q isn't something this can group by or filter on. Pick one of %s", strings.TrimSpace(s), dimList())
}

// validate holds a hand-built Spec to what the parser would have allowed, so a caller that skipped parsing gets the
// same message rather than a confidently empty answer.
func (s Spec) validate() error {
	for _, dim := range s.GroupBy {
		if _, err := ParseDim(string(dim)); err != nil {
			return err
		}
	}
	for _, clause := range s.Where {
		if _, err := ParseDim(string(clause.Field)); err != nil {
			return err
		}
		if len(clause.Values) == 0 {
			return fmt.Errorf("the filter on %s has nothing to match on. Give it one or more values", clause.Field)
		}
	}
	if s.Top < 0 {
		return fmt.Errorf("a top of %d would keep nothing. Pass a positive number of groups to keep, or zero to keep all of them", s.Top)
	}
	return nil
}

// names says the query mentions a dimension, whether to group by it or to filter on it.
func (s Spec) names(dim Dim) bool {
	for _, grouped := range s.GroupBy {
		if grouped == dim {
			return true
		}
	}
	for _, clause := range s.Where {
		if clause.Field == dim {
			return true
		}
	}
	return false
}

// NeedsLanes says the query can only be answered from a session's per-lane detail, because it groups by or filters on
// a lane or an agent. A caller loads the cheaper per-session summary for everything else, which is what keeps a corpus
// scan to a few megabytes.
func (s Spec) NeedsLanes() bool {
	for dim := range laneDims {
		if s.names(dim) {
			return true
		}
	}
	return false
}

// aboutTools says the query names a dimension that only means something on a tool row, which is what decides whether
// the composing rows come along. See Run.
func (s Spec) aboutTools() bool {
	for dim := range toolDims {
		if s.names(dim) {
			return true
		}
	}
	return false
}

// matches says one cell's value satisfies the clause.
func (c Clause) matches(value string) bool {
	for _, pattern := range c.Values {
		if globMatch(pattern, value) {
			return true
		}
	}
	return false
}

// globMatch compares a filter's value against a cell's, ignoring case throughout: a person typing `CHECKER` means the
// same thing as `checker`, and the engine's own names are inconsistently cased (`Bash (git)`, `codegraph (MCP)`).
//
// A leading or trailing `*` is the only wildcard: `codegraph*` is a prefix, `*(mcp)` a suffix, `*check*` a contains,
// and everything else has to match exactly. A `*` anywhere else is a literal one, because a full glob dialect is more
// than a filter flag needs and a tool's name can carry the character.
func globMatch(pattern, value string) bool {
	p, v := strings.ToLower(strings.TrimSpace(pattern)), strings.ToLower(value)
	openStart := strings.HasPrefix(p, "*")
	if openStart {
		p = p[1:]
	}
	openEnd := strings.HasSuffix(p, "*")
	if openEnd {
		p = p[:len(p)-1]
	}
	switch {
	case openStart && openEnd:
		return strings.Contains(v, p)
	case openStart:
		return strings.HasSuffix(v, p)
	case openEnd:
		return strings.HasPrefix(v, p)
	default:
		return v == p
	}
}

// splitList reads a comma-separated list, trimming the spaces a shell leaves behind and dropping the empty piece a
// trailing comma makes.
//
// A `\,` is a literal comma, which one value needs: the `waiting, reason unknown` kind carries the separator inside
// it, and without an escape there'd be no way to name it. A glob is usually easier to type (`kind=waiting*` is all
// four waits, `kind=*unknown` is that one).
func splitList(s string) []string {
	var (
		out     []string
		current strings.Builder
	)
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s) && s[i+1] == ',':
			current.WriteByte(',')
			i++
		case s[i] == ',':
			out = appendValue(out, current.String())
			current.Reset()
		default:
			current.WriteByte(s[i])
		}
	}
	return appendValue(out, current.String())
}

func appendValue(out []string, value string) []string {
	if value = strings.TrimSpace(value); value != "" {
		return append(out, value)
	}
	return out
}

// dimList renders the dimensions for a message, so every message offers the same set.
func dimList() string {
	names := make([]string, 0, len(Dims))
	for _, dim := range Dims {
		names = append(names, string(dim))
	}
	return strings.Join(names, ", ")
}
