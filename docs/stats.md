# The `stats` query

One grammar over every session on disk: filter, group, aggregate. `internal/stats` is the engine, `internal/cli` wires
the flags. Read before changing a dimension or a measure.

## Shape

```sh
claude-session-analyzer stats [<session-id>] [flags]
```

- No session id means corpus scope: every session, narrowed by `--since`, `--until`, `--project`.
- `--where <field>=<value>[,<value>]`, repeatable. Comma is OR within a field, repeated flag is AND across fields.
  Matching is case-insensitive; a leading or trailing `*` globs.
- **A value holding a comma needs `\,`**, since the comma is the separator. Two kind names have one, so
  `kind=waiting\, reason unknown`. Usually `kind=waiting*` is what's wanted anyway: it covers all four waits.
- Quote a glob in a shell that expands one: `--where 'group=codegraph*'`.
- `--group-by <dim>[,<dim>]`. Order is the order the keys come back in.
- `--top N` bounds the output, `--json` for the machine-readable answer, `--no-cache` to bypass `docs/cache.md`.
- `--vocabulary` prints every valid dimension, kind, and class. An agent guessing between `checker` and `checker-script`
  guesses wrong.

## Dimensions

Both the `--group-by` values and the `--where` fields:

`kind`, `class`, `group`, `leaf`, `tool`, `day`, `lane`, `agent`, `session`, `project`.

- `kind` is one of the 11 in `internal/timeline/kind.go`. Four of them are waits.
- `class` is what work a call was doing, `group` is the slice a breakdown draws (`Bash (checker)`, `codegraph (MCP)`),
  `leaf` is the exact thing inside it (`pnpm check`, `codegraph_search`), `tool` is the raw harness name. Rules:
  `docs/timeline-rules.md`.
- `day` is a local date. A session spanning three days contributes to three, split at local midnight, so "July" is exact
  even when a session straddles the boundary.
- `lane` and `agent` need tier two of the cache and cost more to load. A query naming neither never reads it.

## The three denominators

Every answer carries all three, because presenting one as another is the mistake this tool exists to prevent.

- **`wallClockSeconds`**: how long the sessions took. Summed across sessions in corpus scope.
- **`laneTimeSeconds`**: every lane's rows added up. Larger whenever lanes ran at once: 428,756 s against 276,792 s on
  the reference session.
- **`activeSeconds`**: lane time minus every gap kind (`Kind.IsGap()`), so waiting, stalls, and API errors come out.
  This is "net time the agents spent building it".

`matched` carries `shareOfLaneTime`, `shareOfActive`, and `shareOfWallClock` against those.

## Measures

`matched` and every group carry the same set: `seconds`, `rows`, `calls`, `lanes`, `sessions`, `errors`, `timedOut`.

- `calls` counts tool runs, one per call rather than two. See the trap below.
- **`lanes` and `sessions` don't sum.** Distinct counts, not totals. A session contributing to two groups counts once in
  each, so a group's `sessions` is never the sum of a finer roll-up's. `lanes` counted per session, added across them,
  because a lane belongs to one session.
- `matched.sessions` over `totals.sessions` answers "codegraph used in 12 of 735 sessions".
- CLI table shows a `Sessions` column whenever it can differ per row: not with `--group-by session` (1 everywhere), not
  for one session in scope.

## The trap: two rows per call

Every tool call leaves two rows, the agent composing it and the tool running, and **both carry the tool's name**. A
filter on `class`, `group`, `leaf`, or `tool` that takes both reports the tool as costing roughly double, and the answer
looks entirely reasonable.

So a tool question counts only the row the tool ran in (`agg.ToolRuns`). It fires on `--group-by` as well as `--where`:
grouping by `group` with no filter would otherwise fold each composing row into the same key and inflate it.
`Spec.IncludeComposingRows` opts out, and every answer carries a note saying which way it went.

Consequence worth knowing: a tool question grouped by `kind` reports `tool execution` and `stalled`, never `tool call`.
That's correct.

## Worked examples

Checker time, and how many runs:

```sh
claude-session-analyzer stats 532ac591 --where class=checker --group-by leaf --json
```

Whether the agents reached for codegraph or for grep, over a year:

```sh
claude-session-analyzer stats --since 2026-01-01 --where class=search,mcp --group-by group --top 15
```

Wait-loop time per project, and how many sessions each covers:

```sh
claude-session-analyzer stats --where class=wait --group-by project --top 8
```

Net time building one session, excluding waiting on a person or a teammate: `totals.activeSeconds`, or the full split:

```sh
claude-session-analyzer stats 532ac591 --group-by kind
```

Sessions in July, and how long each ran (no timeline parse, so it's instant):

```sh
claude-session-analyzer sessions --json --since 2026-07-01 --until 2026-07-31 --limit 0
```

## What it can't answer

The cube holds no rows, so nothing filters on `info` text or on a single row's timestamps. Those need
`timeline <id> --json --rows` or the CSV. A query asking for a dimension the loaded cells don't carry says so in `notes`
rather than quietly answering something narrower.
