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

## The denominators

Every answer carries four, because presenting one as another is the mistake this tool exists to prevent. Three are the
ladder, each rung the one above minus something; `docs/api.md` is the definition and holds the arithmetic.

- **`laneTimeSeconds`**: every lane's rows added up. Larger than wall clock whenever lanes ran at once: 428,756 s
  against 276,792 s on the reference session.
- **`netSeconds`**: lane time minus `waiting for a person` and `waiting for a teammate`. The agent time the sessions
  actually cost, without counting a teammate's work twice.
- **`activeSeconds`**: net minus the gaps net keeps (stalls, API errors, background-task waits, unknown waits), which is
  lane time minus every `Kind.IsGap()`. "How much was producing", which net doesn't answer and doesn't replace.
- **`wallClockSeconds`**: how long the sessions took. Summed across sessions in corpus scope, so two that ran side by
  side count their overlap twice: honest for one session, loose for a corpus. Not a rung.

`matched` carries `shareOfLaneTime`, `shareOfNet`, `shareOfActive`, and `shareOfWallClock` against those, all over
`matched.seconds`.

**A group's share is against lane time and nothing else.** Groups partition lane time, so an unfiltered column adds to
100%. Against net, a `waiting for a person` group would read as 93% of a total it's excluded from, and that reading is a
bug this repo already fixed in the other direction. The CLI table names the denominator in the header.

## Measures

`matched` and every group carry the same set: `seconds`, `composingSeconds`, `stalledSeconds`, `rows`, `calls`, `lanes`,
`sessions`, `errors`, `timedOut`.

- `calls` counts tool runs, one per call rather than two. A stalled call is one of them. See the trap below.
- `seconds`, `composingSeconds`, and `stalledSeconds` are a call's three clocks. What they hold, and how they relate to
  each other, depends on `toolClocksApart`: the trap below.
- **`lanes` and `sessions` don't sum.** Distinct counts, not totals. A session contributing to two groups counts once in
  each, so a group's `sessions` is never the sum of a finer roll-up's. `lanes` counted per session, added across them,
  because a lane belongs to one session.
- `matched.sessions` over `totals.sessions` answers "codegraph used in 12 of 735 sessions".
- CLI table shows a `Sessions` column whenever it can differ per row: not with `--group-by session` (1 everywhere), not
  for one session in scope.

## The trap: one call, three clocks

Every tool call leaves two rows, the agent composing it and the tool running, and **both carry the tool's name**. A
third name lands on the same group when the run row got a `stalled` verdict instead. A filter on `class`, `group`,
`leaf`, or `tool` that adds all of them up reports the tool as costing what the agent and a suspended session cost, and
the answer looks entirely reasonable.

So a tool question keeps the three apart, and says so with `toolClocksApart: true`. `seconds` is the tool running,
`composingSeconds` is the agent writing the call, `stalledSeconds` is the call that came back far too late. It fires on
`--group-by` as well as `--where`: grouping by `group` with no filter would otherwise fold each composing row into the
same key and inflate it. `Spec.IncludeComposingRows` opts out of the split entirely, and every answer carries a note
saying which way it went.

Numbers worth knowing, from `532ac591`, verified 2026-08-08. The split inverts per tool, which is the point of it:

- `Edit`: 1,032 calls, 2.24 h composing, 0.03 h running. The model streams the whole diff as the call's arguments.
- `Bash (checker)`: 344 calls, 0.40 h composing, 7.86 h running.
- `Bash (file write)`: 69 calls, 6.26 h of it one stalled `rm`, leaving 0.1 h across the other 68.

Two consequences:

- **A question that isn't about tools keeps `seconds` whole**, because a breakdown by `kind` or by `day` has to
  partition lane time. There `composingSeconds` and `stalledSeconds` are subsets of `seconds` rather than carved out of
  it, and `toolClocksApart` is `false`. The CLI leaves both columns off, since the `tool call` and `stalled` rows
  already are those numbers.
- **A tool question grouped by `kind` reports a `tool call` row with `seconds: 0`** and its time in `composingSeconds`.
  That's correct: the row is the agent's, not the tool's. Groups are ordered by all three clocks together rather than by
  `seconds`, so a group carrying one long stall stays where `--top` can show it.

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

The agent time one session cost, with waiting on a person or a teammate out of it: `totals.netSeconds`. How much of that
was producing rather than stalled: `totals.activeSeconds`. Either way, the full split beside them:

```sh
claude-session-analyzer stats 532ac591 --group-by kind
```

How much of a tool's time was the tool, and how much was the agent writing the call:

```sh
claude-session-analyzer stats 532ac591 --group-by group --top 8
```

Sessions in July, and how long each ran (no timeline parse, so it's instant):

```sh
claude-session-analyzer sessions --json --since 2026-07-01 --until 2026-07-31 --limit 0
```

## What it can't answer

The cube holds no rows, so nothing filters on `info` text or on a single row's timestamps. Those need
`timeline <id> --json --rows` or the CSV. A query asking for a dimension the loaded cells don't carry says so in `notes`
rather than quietly answering something narrower.
