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

## The trap: two rows per call

Every tool call leaves two rows, the agent composing it and the tool running, and **both carry the tool's name**. A
filter on `class`, `group`, `leaf`, or `tool` that takes both reports the tool as costing roughly double, and the answer
looks entirely reasonable.

So a tool filter counts only the row the tool ran in (`agg.ToolRuns`). Consequence worth knowing: a query filtering on a
tool dimension and grouping by `kind` reports `tool execution` and `stalled`, never `tool call`. That's correct.

## Worked examples

Checker time, and how many runs:

```sh
claude-session-analyzer stats 532ac591 --where class=checker --group-by leaf --json
```

Whether the agents reached for codegraph or for grep, over a year:

```sh
claude-session-analyzer stats --since 2026-01-01 --where class=search,mcp --group-by group --top 15
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
