package stats_test

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/vdavid/claude-session-analyzer/internal/stats"
	"github.com/vdavid/claude-session-analyzer/internal/timeline"
)

func TestParsingTheFlagsACLIHands(t *testing.T) {
	spec, err := stats.ParseSpec([]string{"class=checker,test", "day=2026-08-03"}, "kind,day", 5)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if got, want := len(spec.Where), 2; got != want {
		t.Fatalf("clauses = %d, want %d", got, want)
	}
	if got, want := spec.Where[0].Field, stats.DimClass; got != want {
		t.Errorf("first field = %q, want %q", got, want)
	}
	if got, want := spec.Where[0].Values, []string{"checker", "test"}; !slices.Equal(got, want) {
		t.Errorf("first values = %q, want %q", got, want)
	}
	if got, want := spec.GroupBy, []stats.Dim{stats.DimKind, stats.DimDay}; !slices.Equal(got, want) {
		t.Errorf("group by = %q, want %q", got, want)
	}
	if got, want := spec.Top, 5; got != want {
		t.Errorf("top = %d, want %d", got, want)
	}
}

func TestParsingIsForgivingAboutCaseSpacingAndAnEmptyGroupBy(t *testing.T) {
	spec, err := stats.ParseSpec([]string{" Class = checker , test "}, "", 0)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if len(spec.Where) != 1 {
		t.Fatalf("clauses = %d, want 1", len(spec.Where))
	}
	if got, want := spec.Where[0].Field, stats.DimClass; got != want {
		t.Errorf("field = %q, want %q", got, want)
	}
	if got, want := spec.Where[0].Values, []string{"checker", "test"}; !slices.Equal(got, want) {
		t.Errorf("values = %q, want %q", got, want)
	}
	if len(spec.GroupBy) != 0 {
		t.Errorf("group by = %q, want nothing", spec.GroupBy)
	}
}

// One activity kind carries the separator inside it, so without an escape there'd be no way to filter on it.
func TestAnEscapedCommaIsPartOfTheValue(t *testing.T) {
	clause, err := stats.ParseClause(`kind=thinking,waiting\, reason unknown`)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	want := []string{"thinking", "waiting, reason unknown"}
	if !slices.Equal(clause.Values, want) {
		t.Errorf("values = %q, want %q", clause.Values, want)
	}
	if !slices.Contains(stats.Vocabulary().Kinds, clause.Values[1]) {
		t.Errorf("%q should be one of the kinds the vocabulary offers", clause.Values[1])
	}
}

func TestGroupingByTheSameDimensionTwiceGroupsByItOnce(t *testing.T) {
	dims, err := stats.ParseGroupBy("kind,kind,day,")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if got, want := dims, []stats.Dim{stats.DimKind, stats.DimDay}; !slices.Equal(got, want) {
		t.Errorf("group by = %q, want %q", got, want)
	}
}

// The messages are read by agents and by people, and both need to know what to type instead.
func TestAMalformedQuerySaysWhatWasWrongAndWhatToTypeInstead(t *testing.T) {
	cases := []struct {
		name     string
		parse    func() error
		mentions []string
	}{
		{
			name:     "unknown dimension to group by",
			parse:    func() error { _, err := stats.ParseGroupBy("kindz"); return err },
			mentions: []string{"kindz", "kind", "class", "session", "project"},
		},
		{
			name:     "unknown dimension to filter on",
			parse:    func() error { _, err := stats.ParseClause("clas=checker"); return err },
			mentions: []string{"clas", "class"},
		},
		{
			name:     "no equals sign",
			parse:    func() error { _, err := stats.ParseClause("checker"); return err },
			mentions: []string{"checker", "="},
		},
		{
			name:     "nothing to match",
			parse:    func() error { _, err := stats.ParseClause("class="); return err },
			mentions: []string{"class"},
		},
		{
			name:     "a top that keeps nothing",
			parse:    func() error { _, err := stats.ParseSpec(nil, "kind", -1); return err },
			mentions: []string{"-1"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.parse()
			if err == nil {
				t.Fatalf("wanted a message saying what to do instead")
			}
			for _, want := range c.mentions {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("message %q should mention %q", err, want)
				}
			}
			for _, banned := range []string{"error", "failed", "invalid", "illegal"} {
				if strings.Contains(strings.ToLower(err.Error()), banned) {
					t.Errorf("message %q shouldn't say %q", err, banned)
				}
			}
		})
	}
}

// Only a lane question is worth loading the per-lane detail for, which is a tenfold read over a corpus.
func TestOnlyALaneOrAgentQueryNeedsThePerLaneDetail(t *testing.T) {
	cases := []struct {
		name string
		spec stats.Spec
		want bool
	}{
		{"grouping by lane", stats.Spec{GroupBy: []stats.Dim{stats.DimLane}}, true},
		{"grouping by agent", stats.Spec{GroupBy: []stats.Dim{stats.DimAgent}}, true},
		{"filtering on a lane", stats.Spec{Where: []stats.Clause{where(stats.DimLane, "lead")}}, true},
		{"grouping by tool", stats.Spec{GroupBy: []stats.Dim{stats.DimGroup, stats.DimDay}}, false},
		{"asking nothing in particular", stats.Spec{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.spec.NeedsLanes(); got != c.want {
				t.Errorf("needs lanes = %v, want %v", got, c.want)
			}
		})
	}
}

func TestTheVocabularyIsEverythingAQueryCanName(t *testing.T) {
	vocab := stats.Vocabulary()

	if got, want := vocab.Dims, stats.Dims; !slices.Equal(got, want) {
		t.Errorf("dimensions = %q, want %q", got, want)
	}
	if got, want := len(vocab.Kinds), len(timeline.Kinds); got != want {
		t.Fatalf("kinds = %d, want %d", got, want)
	}
	for i, kind := range timeline.Kinds {
		if vocab.Kinds[i] != string(kind) {
			t.Errorf("kind %d = %q, want %q", i, vocab.Kinds[i], kind)
		}
	}
	for _, want := range []string{"checker", "mcp", "dev server", "file read", "other"} {
		if !slices.Contains(vocab.Classes, want) {
			t.Errorf("classes should carry %q, got %q", want, vocab.Classes)
		}
	}
}

// The vocabulary reads `timeline.Classes`, and that list is hand-written beside the constants it names. Reading the
// engine's source is what holds the two together: declare a class there without adding it to `Classes` and this fails
// with the name that's missing.
func TestTheClassListMatchesTheEngines(t *testing.T) {
	source, err := os.ReadFile("../timeline/tool.go")
	if err != nil {
		t.Fatalf("reading the engine's classes: %v", err)
	}
	matches := regexp.MustCompile(`Class\w+\s+ToolClass = "([^"]+)"`).FindAllStringSubmatch(string(source), -1)
	if len(matches) < 10 {
		t.Fatalf("found %d classes in tool.go, which means the shape it declares them in moved", len(matches))
	}

	var declared []string
	for _, m := range matches {
		declared = append(declared, m[1])
	}
	if got := stats.Vocabulary().Classes; !slices.Equal(got, declared) {
		t.Errorf("the class list has drifted from the engine's.\n got: %q\nwant: %q", got, declared)
	}
}
