# stats/: one grammar over the cube

Filter, group, add up. Every question an agent or a person asks about where the time went is the same sum over
`internal/agg`'s cells, differing only in which cells it keeps and which dimensions it keeps them by. Ten dimensions
(`kind`, `class`, `group`, `leaf`, `tool`, `day`, `lane`, `agent`, `session`, `project`), and they're both the
`--group-by` values and the `--where` fields, so a caller learns one vocabulary.

## Must-knows

- **A tool question counts only the rows a tool ran in.** Naming `class`, `group`, `leaf`, or `tool` anywhere in a query
  puts the cells through `agg.ToolRuns` first. Every call leaves two rows, the agent composing it and the tool running,
  and both carry the tool's name, so a total that takes them all reports the checker as costing about twice what it did
  and the answer looks perfectly reasonable. `TestAToolFilterCountsOnlyTheRowsTheToolRanIn` holds it.
- **That rule takes the `tool call` kind with it, on purpose.** A query filtering on a class and grouping by kind
  reports `tool execution` and `stalled` and no `tool call`, because that row is the agent's rather than the tool's. A
  note in the answer says so, and `TestAToolFilterGroupedByKindHasNoToolCallKindLeft` pins it.
  `Spec.IncludeComposingRows` is the opt-out, for the rare question about the agent instead.
- **The denominators are the unfiltered whole.** `Totals` covers every session in scope with no clause applied and no
  tool-run filter, so a share says what part of the session went somewhere rather than what part of the filter did.
  Three of them, and they answer different questions: lane time is every lane's clock added up, active is lane time with
  every gap taken out, and wall clock is first record to last. Wall clock is summed across sessions, so two that ran
  side by side count their overlap twice: it's honest for one session and loose for a corpus.
- **Lanes are counted per session and added up across them.** A lane belongs to one session, so two sessions with three
  lanes each are six. Cells that have already been summed past the lane dimension carry a count instead of a name, and
  the largest count seen is all that evidence supports, which makes the number a lower bound when two tool groups were
  used by different lanes.
- **Say so rather than answering something narrower.** A query naming `lane` or `agent` against cells summed without
  them leaves a `Result.Notes` entry pointing at the per-lane detail. `Spec.NeedsLanes()` is how a caller knows to load
  it before asking.
- **Don't import `internal/cache`.** `Source` is the adapter, and the CLI fills it from a digest or, for a lane
  question, from the detail. This package stays a pure function over cells.
- **The class list is a copy.** The engine declares its 15 tool classes in `internal/timeline/tool.go` and exports no
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
- `stats.go`: the answer. `Source` in, `Result` out. `Totals`, `Matched`, and `Group` share `Measures`, and `Key.Value`
  reads one dimension off a group so a caller renders a column per grouped dimension without a switch of its own.
- Lane time, active seconds, and the rounding come from `internal/report`, so the denominator here is the same number
  the API reports for the same sessions.
