# stats/: one grammar over the cube

Filter, group, add up. Every question an agent or a person asks about where the time went is the same sum over
`internal/agg`'s cells, differing only in which cells it keeps and which dimensions it keeps them by. Ten dimensions
(`kind`, `class`, `group`, `leaf`, `tool`, `day`, `lane`, `agent`, `session`, `project`), and they're both the
`--group-by` values and the `--where` fields, so a caller learns one vocabulary.

## Must-knows

- **A tool question keeps a call's three clocks apart.** Naming `class`, `group`, `leaf`, or `tool` anywhere in a query
  sets `Result.ToolClocksApart`: `Seconds` becomes the tool running, `ComposingSeconds` the agent writing the call, and
  `StalledSeconds` a call that came back far too late. All three arrive on a cell carrying the tool's name, so one
  number holding them reports the checker as costing what the agent and a suspended session cost, and the answer looks
  perfectly reasonable. `TestAToolFilterCountsOnlyTheRowsTheToolRanIn` and
  `TestAStalledCallIsReportedApartFromWhatTheToolCost` hold it. `Spec.IncludeComposingRows` opts out of the split.
- **Rows and calls follow the split.** A composing row is neither a row nor a call of a tool question; a stalled run is
  both, because it was a call. Cells carrying no tool are dropped entirely (`agg.Cell.IsAboutATool`), or the answer
  grows a nameless group holding the session's thinking and waiting.
- **On a tool question the `tool call` kind comes back with `Seconds: 0`** and its time in `ComposingSeconds`, which is
  correct: the row is the agent's rather than the tool's, and it isn't dropped any more.
  `TestAToolQuestionReportsComposingTimeBesideTheToolsOwnClockRatherThanDroppingIt` pins both halves.
- **Anywhere else `Seconds` stays whole**, because a breakdown by `kind` or by `day` has to partition lane time. There
  the other two measures are subsets of `Seconds` rather than carved out of it, which is what `ToolClocksApart: false`
  tells a renderer.
- **`Group.weight` is what "biggest first" and `Top` mean**, and on a tool question it's all three clocks. Ordering by
  `Seconds` alone sinks a group that spent its time composing (`Edit`) or stalling (`Bash (file write)`) below groups
  costing a fraction of it, and `--top 8` then hides the rows worth looking at.
- **The denominators are the unfiltered whole.** `Totals` covers every session in scope with no clause applied and no
  clock split, so a share says what part of the session went somewhere rather than what part of the filter did. Four of
  them: the ladder (lane time, net, active) from `report.TotalsFrom`, plus wall clock. Wall clock is summed across
  sessions, so two that ran side by side count their overlap twice: honest for one session and loose for a corpus.
  Definition of the ladder: `docs/api.md`.
- **A group's share is against lane time only.** Groups partition lane time, so an unfiltered column adds to 100%.
  Against net, a `waiting for a person` group reads as 93% of a total it's excluded from.
- **Lanes are counted per session and added up across them.** A lane belongs to one session, so two sessions with three
  lanes each are six. Cells that have already been summed past the lane dimension carry a count instead of a name, and
  the largest count seen is all that evidence supports, which makes the number a lower bound when two tool groups were
  used by different lanes.
- **`Measures.Sessions` is a distinct count, so it doesn't sum either.** A session contributing to two groups counts
  once in each, and a group's count is never the sum of a finer roll-up's. `Matched.Sessions` over `Totals.Sessions` is
  "codegraph used in 12 of 735 sessions". `TestASessionCountIsDistinctRatherThanSummableAcrossGroups` holds it.
- **Say so rather than answering something narrower.** A query naming `lane` or `agent` against cells summed without
  them leaves a `Result.Notes` entry pointing at the per-lane detail. `Spec.NeedsLanes()` is how a caller knows to load
  it before asking.
- **Don't import `internal/cache`.** `Source` is the adapter, and the CLI fills it from a digest or, for a lane
  question, from the detail. This package stays a pure function over cells.
- **The class list is a copy.** The engine declares its 16 tool classes in `internal/timeline/tool.go` and exports no
  list of them, so `classes` in `spec.go` is a duplicate. `TestTheClassListMatchesTheEngines` reads that file and fails
  with the name to add here.
- **A `\,` is a literal comma in a filter value.** The `waiting, reason unknown` kind carries the separator inside it,
  and would otherwise be the one value nobody can type. A glob is usually easier: `kind=waiting*` is all four waits.
- **Messages are sentences with a next step in them**, and they never say "error" or "failed". They're read by agents as
  often as by people, and an agent that can't tell what to type next guesses. Every message about a dimension lists the
  valid ones.
- **A rule is test-first here.** Write the failing case, watch it fail for the right reason, then implement.

## Module map

- `spec.go`: the question. `Dim` and `Dims`, `Spec`, `Clause`, the parsers a CLI hands raw flag strings to (`ParseSpec`,
  `ParseGroupBy`, `ParseClause`, `ParseDim`), matching (case-insensitive, with a leading or trailing `*` the only
  wildcard), and `Vocabulary`.
- `stats.go`: the answer. `Source` in, `Result` out. `Totals`, `Matched`, and `Group` share `Measures`, `Key.Value`
  reads one dimension off a group so a caller renders a column per grouped dimension without a switch of its own, and
  `acc.add` is where the clock split happens.
- The ladder and the rounding come from `internal/report`, so the denominators here are the same numbers the API reports
  for the same sessions.
