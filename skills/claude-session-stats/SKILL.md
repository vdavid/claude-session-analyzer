---
name: claude-session-stats
description: >
    Answer questions about David's own Claude Code sessions from the transcripts on this machine: where time went, how
    long a session or a tool took, which tools the agents reached for, how much time went into waiting (on him, on
    teammates, on background tasks), net agent time and active working time with the waiting excluded, how many sessions
    in a period and how long they ran, and whether agents preferred one tool over another. Use whenever a question is
    about past Claude Code sessions, agents, subagents, tool use, or where the time went. Also use for "how much time
    did I spend on X", "did my agents use codegraph", "how long was I waiting", "how many sessions last month".
---

# Claude session stats

`claude-session-analyzer` reads Claude Code transcripts under `~/.claude/projects` and reconstructs where time went, per
agent, second by second. Source: `~/projects-git/vdavid/claude-session-analyzer`.

Binary on PATH. Missing? `cd ~/projects-git/vdavid/claude-session-analyzer && go install ./cmd/claude-session-analyzer`.

## Before a corpus question

```sh
claude-session-analyzer cache warm
```

First run parses everything: 3.9 s over 735 sessions (measured 2026-08-07). After that only changed sessions reparse,
and a corpus query is about 0.25 s. Single-session questions need no warm.

## The ladder, and wall clock beside it

Three durations over the same rows, each one the rung above minus something. Say which rung you're quoting. Real
numbers, one session, `532ac591`:

```
lane time  119h05m  every agent's clock added up
  net       45h11m  minus waiting for a person, minus waiting for a teammate
  active    38h20m  minus stalls, API errors, background-task waits, unknown waits
```

- **lane time** (`laneTimeSeconds`): bigger than wall clock whenever agents ran at once. A breakdown by activity or by
  tool is a breakdown of lane time.
- **net** (`netSeconds`): the agent time the work actually cost. Waiting on a teammate is already counted as that
  teammate's own lane time, so keeping it counts the same work twice, and waiting on David was never agent time. Stalls,
  API errors, background-task waits, and compacting all stay in.
- **active** (`activeSeconds`): how much was producing. Different question from net, not a better answer: net on that
  session holds a 6h15m stalled `rm` that active doesn't.
- **wall clock** (`wallClockSeconds`): how long the session took, first record to last. Not a rung.

All four are fields. Never compute one by hand from the pie.

For "how much time did this cost", quote net. For "how much of it was real work", quote active. For "how long did this
take", quote wall clock.

## Commands

Find the session:

```sh
claude-session-analyzer sessions --limit 10
claude-session-analyzer sessions --json --project cmdr --since 2026-07-01 --until 2026-07-31 --limit 0
```

Any unique id prefix works (`532ac591`). `--limit 0` for all. Session-level questions ("how many sessions in July, how
long were they") need only this: it reads the two ends of each transcript, so it's instant and needs no cache.

One session, everything summed:

```sh
claude-session-analyzer timeline 532ac591 --json
```

Filter, group, aggregate, over one session or the whole corpus:

```sh
claude-session-analyzer stats [<session-id>] \
  --where <field>=<value>[,<value>] \
  --group-by <dim>[,<dim>] \
  --top 20 --json
```

- Comma is OR inside a field, repeated `--where` is AND across fields. Case-insensitive, `*` globs. Quote globs so the
  shell doesn't eat them: `--where 'group=codegraph*'`.
- All waiting at once: `--where kind=waiting*`. A value with a comma in it needs `\,`.
- No session id means every session; narrow with `--since`, `--until`, `--project`.
- Dimensions: `kind`, `class`, `group`, `leaf`, `tool`, `day`, `lane`, `agent`, `session`, `project`.
- `--vocabulary` lists every valid value. Read it rather than guessing between `checker` and `checker-script`.

## Worked answers

Time and runs spent on the checker script, plus its share:

```sh
claude-session-analyzer stats 532ac591 --where class=checker --group-by leaf --json
```

Read `matched.seconds`, `matched.calls`, `matched.shareOfLaneTime`.

How much of a tool's time was the tool, and how much was the agent writing the call:

```sh
claude-session-analyzer stats 532ac591 --group-by group --top 8
```

Three clocks per call, never added together: `seconds` is the tool running, `composingSeconds` is the agent writing the
call, `stalledSeconds` is a call that came back far too late to have been running. The split inverts per tool, so read
the one the question asks for. On that session `Edit` is 2.24 h composing against 0.03 h running over 1,032 calls, while
`Bash (checker)` is 0.40 h composing against 7.86 h running over 344, and `Bash (file write)` is 6.26 h of one stalled
`rm` plus 0.1 h across its other 68 calls.

codegraph vs grep, across the year:

```sh
claude-session-analyzer stats --since 2026-01-01 --where class=search,mcp --group-by group --top 15
```

Where a session's time went, plus the whole ladder under the table:

```sh
claude-session-analyzer stats 532ac591 --group-by kind
```

Which agents used a tool:

```sh
claude-session-analyzer stats 532ac591 --where 'group=codegraph*' --group-by agent
```

How widely, not just how often. `matched.sessions` over `totals.sessions` answers "codegraph in 12 of 735 sessions":

```sh
claude-session-analyzer stats --where 'group=codegraph*' --group-by project --json
```

## Rules

- **Bound the output.** Always `--top`, always a `--limit`. Never `timeline --json --rows` on a big session: 8 MB of
  JSON into context. Rows belong in a CSV: `timeline <id> --out /tmp/t.csv`.
- **A tool's own clock excludes the agent composing the call, and excludes a stall.** The CLI already splits them into
  `seconds`, `composingSeconds`, and `stalledSeconds`; never add two of them together, and say which one a number is.
- **Quote the denominator** with any percentage.
- **`sessions` and `lanes` never sum.** Distinct counts: a session in two groups counts once in each. For a per-session
  rate divide by `matched.sessions`, never `totals.sessions`, or you divide by sessions that never did the thing.
- Only `--json` output is stable enough to parse. Table output is for reading.

## Limits

- No filtering on row text or single-row timestamps: the cache holds sums, not rows. Those need
  `timeline <id> --json --rows` or the CSV.
- A session still being written reparses on every query, which is a second or two on a big one.
- Deeper docs, if a question goes past this: `docs/stats.md` and `docs/cache.md` in the repo.
