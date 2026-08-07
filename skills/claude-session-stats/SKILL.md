---
name: claude-session-stats
description: >
    Answer questions about David's own Claude Code sessions from the transcripts on this machine: where time went, how
    long a session or a tool took, which tools the agents reached for, how much time went into waiting (on him, on
    teammates, on background tasks), net working time with waiting excluded, how many sessions in a period and how long
    they ran, and whether agents preferred one tool over another. Use whenever a question is about past Claude Code
    sessions, agents, subagents, tool use, or where the time went. Also use for "how much time did I spend on X", "did
    my agents use codegraph", "how long was I waiting", "how many sessions last month".
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

## Three numbers, never interchangeable

Say which one you're quoting.

- **wall clock**: how long the session took.
- **lane time**: every agent's clock added up. Bigger whenever agents ran at once (405,702 s against 264,524 s wall
  clock on one real session). A breakdown by activity is a breakdown of lane time.
- **active**: lane time minus every gap (waiting on a person, a teammate, a background task, plus stalls and API
  errors). This is "net time the agents spent building it".

`activeSeconds` is a field. Never compute it by hand from the pie.

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

- Comma is OR inside a field, repeated `--where` is AND across fields. Case-insensitive, `*` globs.
- No session id means every session; narrow with `--since`, `--until`, `--project`.
- Dimensions: `kind`, `class`, `group`, `leaf`, `tool`, `day`, `lane`, `agent`, `session`, `project`.
- `--vocabulary` lists every valid value. Read it rather than guessing between `checker` and `checker-script`.

## Worked answers

Time and runs spent on the checker script, plus its share:

```sh
claude-session-analyzer stats 532ac591 --where class=checker --group-by leaf --json
```

Read `matched.seconds`, `matched.calls`, `matched.shareOfActive`.

codegraph vs grep, across the year:

```sh
claude-session-analyzer stats --since 2026-01-01 --where class=search,mcp --group-by group --top 15
```

Where a session's time went, and net working time:

```sh
claude-session-analyzer stats 532ac591 --group-by kind
```

Which agents used a tool:

```sh
claude-session-analyzer stats 532ac591 --where group=codegraph* --group-by agent
```

## Rules

- **Bound the output.** Always `--top`, always a `--limit`. Never `timeline --json --rows` on a big session: 8 MB of
  JSON into context. Rows belong in a CSV: `timeline <id> --out /tmp/t.csv`.
- **A tool's own clock excludes the agent composing the call.** The CLI already handles this; don't add the two together
  from raw rows.
- **Quote the denominator** with any percentage.
- Only `--json` output is stable enough to parse. Table output is for reading.

## Limits

- No filtering on row text or single-row timestamps: the cache holds sums, not rows. Those need
  `timeline <id> --json --rows` or the CSV.
- A session still being written reparses on every query, which is a second or two on a big one.
- Deeper docs, if a question goes past this: `docs/stats.md` and `docs/cache.md` in the repo.
