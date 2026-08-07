# Claude session analyzer

[![CI](https://github.com/vdavid/claude-session-analyzer/actions/workflows/ci.yml/badge.svg)](https://github.com/vdavid/claude-session-analyzer/actions/workflows/ci.yml)
[![License: MIT OR Apache-2.0](https://img.shields.io/badge/license-MIT%20OR%20Apache--2.0-blue.svg)](LICENSE-MIT)

Reconstructs where the time went in a Claude Code session: per agent, second by second, from the first prompt to the
last tool result.

Scrolling a transcript tells you what happened. It doesn't tell you that a three-day multi-agent effort spent most of
its wall clock with the lead idle, or that one agent sat suspended for six hours on an `rm`. This reads the transcripts
Claude Code already wrote to your disk and answers that.

Two surfaces over one engine. A command line that writes a CSV, answers in JSON, and takes queries: "how much of this
session went into the checker script, and in how many runs?", "did my agents reach for codegraph or for grep this
year?". And a local web app with a time-spent donut, an agent-liveness swimlane, a breakdown of the tools the agents
used, and every derived row in a sortable sheet.

## Status

Early, and it works. The engine, the command line, the HTTP API, and the web app are in. It's a personal tool, published
because nothing in it is personal.

## What you need

Go, Node, and pnpm, all pinned in `.mise.toml` if you use [mise](https://mise.jdx.dev). Plus some Claude Code sessions:
the tool reads `~/.claude/projects/`, or `$CLAUDE_CONFIG_DIR/projects` when that's set. Nothing is uploaded, and nothing
is ever written back to your transcripts. Querying across every session keeps a summary of each one under
`~/.cache/claude-session-analyzer/`, which you can delete at any time.

## Quick start

The web app, both halves at once:

```sh
pnpm install
pnpm dev            # the Go server and the frontend, on the ports in `.env`
```

Then open http://127.0.0.1:19428. The front page lists every session on the machine; opening one shows where its time
went. Both services bind to `127.0.0.1` and nothing else: a tool that reads every transcript on your machine has no
business on the network.

The command line:

```sh
go install ./cmd/claude-session-analyzer

claude-session-analyzer sessions                 # what's on disk, newest first
claude-session-analyzer sessions --json --since 2026-07-01 --limit 0
claude-session-analyzer timeline 532ac591        # the CSV, to standard output
claude-session-analyzer timeline 532ac591 --json # the same session, already summed
claude-session-analyzer serve                    # the API alone, on http://127.0.0.1:19427
```

A session id can be any prefix that matches one session, which is why `532ac591` works. `sessions` lists 735 sessions
and 3.8 GB in well under a second, because it reads the two ends of each transcript rather than any of the middle.
`--root` points any command somewhere other than the default.

## Asking questions

`stats` is one grammar over one session or over every session you have: filter, group, add up.

```sh
claude-session-analyzer cache warm               # once, 3.9 s over 735 sessions

# Where did one session's time go, and how much was actual work?
claude-session-analyzer stats 532ac591 --group-by kind

# How long did the checker script take, over how many runs, and what share of the time?
claude-session-analyzer stats 532ac591 --where class=checker --group-by leaf

# Did the agents reach for codegraph or for grep this year?
claude-session-analyzer stats --since 2026-01-01 --where class=search,mcp --group-by group

# Which agents used a given tool?
claude-session-analyzer stats --where 'group=codegraph*' --group-by agent

# How much of a tool's time was the tool, and how much was the agent writing the call?
claude-session-analyzer stats 532ac591 --group-by group --top 8
```

Dimensions are `kind`, `category`, `class`, `group`, `leaf`, `tool`, `day`, `lane`, `agent`, `session`, and `project`,
for both `--group-by` and `--where`. `--vocabulary` prints every valid value. `--json` gives the machine-readable
answer.

Tool calls come at two levels of detail: `category` is seven buckets (management, read, write, build, checks, QA,
other), which is the level people ask at, and `class` is the sixteen underneath them. `--group-by category` is the quick
answer to "where did the tool calls actually go".

Every answer carries four denominators, because how long a session took, how much agent time it cost, how much of that
was actually producing, and every agent's clock added together are four different numbers. The last three are printed as
a ladder, each one the rung above minus something, so it's clear what a percentage divided by.

Warming keeps a summary of each session under `~/.cache/claude-session-analyzer/` (33 MB against 3.8 GB of transcripts),
so a query across everything takes about a quarter of a second. `cache info` says what's stored and `cache clear`
removes it.

## Pointing your own agents at it

`skills/claude-session-stats/` is a Claude Code skill that teaches an agent this vocabulary, so "how much of yesterday
was me keeping agents waiting?" reaches for the CLI on its own. Install it by linking it in:

```sh
ln -s "$PWD/skills/claude-session-stats" ~/.claude/skills/claude-session-stats
```

It's a plain CLI underneath, so anything that can run a command can use it, with or without the skill.

## The CSV

One row per stretch of one agent's wall clock. Within an agent the rows tile: each starts where the last one ended, so
nothing is unaccounted for and nothing is counted twice.

| Column         | What's in it                                                                                    |
| -------------- | ----------------------------------------------------------------------------------------------- |
| `From`         | When the stretch started, RFC 3339 in UTC, to the millisecond.                                  |
| `Until`        | When it ended, same format.                                                                     |
| `Agent`        | The lane's name: `lead`, or a subagent's name from its metadata. Names repeat, ids don't.       |
| `Activity`     | One of the 11 kinds below.                                                                      |
| `Extra info`   | What the row was about: the command a tool ran, who a wait was on, the reason a request failed. |
| `Duration (s)` | `Until` minus `From`, in seconds to the millisecond.                                            |

The 11 activities:

- **thinking**: the model produced a thinking block. Includes model latency, see the limitations below.
- **writing**: the agent composed prose for whoever called it.
- **tool call**: the agent composed a tool call, up to the moment the call was issued.
- **tool execution**: the tool ran, from the call until its result came back. The honest wall clock of the tool,
  including anything the tool itself waited on.
- **waiting for a person**, **waiting for a teammate**, **waiting for a background task**, and **waiting, reason
  unknown**: the lane produced nothing, named by what ended the gap. Waiting is four values rather than one because "71
  hours of waiting" answers nothing, while knowing that 41 of those hours were on a person does. A gap a background
  task's notification ended counts as waiting for the teammate that started the task, when that teammate was still
  running: the lead sitting idle for 25 minutes was blocked on the agent, not on the tenth-of-a-second poll loop that
  woke it.
- **API error**: the API refused the request. A rate limit, an expired login, an outage.
- **stalled**: a result that arrived far too late for what the call was doing, so the agent was suspended rather than
  working.
- **compacting**: the harness was compacting the context.

What each one means, and every judgement call behind it, is in `docs/timeline-rules.md`.

## What the web app shows

Open a session and it derives the timeline on the spot, then shows:

- **A trace of how many agents were producing at once**, across the whole span. It's the shape of the session in one
  line, including the stretches where nobody was working.
- **Elapsed time beside lane time.** Those are different numbers and the page never mixes them up: lane time adds every
  agent's clock together, so two agents working side by side for an hour is two hours of lane time and one hour of
  elapsed time.
- **Lane time, net agent time, and active time as a ladder**, with the subtraction between the rungs written out. Net is
  lane time minus the waiting whose clock belongs to somebody else, so it's what the session actually cost in agent
  time; active takes the stalls and the API errors out on top of that. They answer different questions, and seeing what
  each one dropped is what stops you quoting one as another.
- **A donut of lane time by activity**, with a legend that spells out what each slice does and doesn't include.
- **A swimlane of who was alive when.** A thick bar means the agent was producing something; a thin one means it wasn't,
  coloured by what it was waiting on. A session with a thousand agents collapses its workflows into a row each, opening
  on click.
- **Tools, as two questions rather than one.** A donut of every tool call in the session answers how often each tool was
  reached for. Beside it, a bar per tool answers where that tool's time went, and the answer flips from tool to tool: an
  `Edit` is 1,032 calls that cost two hours of the model writing diffs and under two minutes of actually writing to
  disk, while a checker run is the other way round. A third stretch shows up when a call came back far too late to have
  been running, which is a suspended agent rather than a slow tool, so one stalled command doesn't get to make `Bash`
  look pathological. Under both, a table with all three numbers per tool, never added together, plus how many agents
  reached for each. `Bash` is broken up by what the command was doing and an MCP server's methods are gathered under the
  server, so "who used codegraph" is one row rather than four names you have to know to look for. Pick a tool anywhere
  and the sheet below filters to its calls.
- **Every derived row**, sortable and filterable, down to sub-second spans.

## How it works

Claude Code writes one JSONL transcript per session under `~/.claude/projects/`, plus one per subagent it spawns. The
engine streams those, pairs tool calls with their results, and turns the record stream into labelled spans. A wait is
attributed to whatever ended it: a person, a teammate, or a background task.

The format is reverse-engineered from transcripts on disk rather than from a published spec, and it drifts between
Claude Code versions, so the parser skips what it doesn't recognize instead of failing. `docs/transcript-format.md`
holds what's known and how each claim was checked.

## Limitations

These are honest, not temporary. A tool that says where your time went has to be believable, so here's where it isn't
certain:

- **Thinking content is unavailable.** Claude Code stores an empty string for reasoning text in almost every transcript,
  so a thinking row can only be labelled by what the agent did next. Those labels start with "before" to mark them as
  inference.
- **Thinking spans include model latency.** A block's timestamp is when it finished streaming, so time in the API queue
  and in prompt processing lands inside the thinking span. On a large context that's a real share of it. The same goes
  for a tool call span with no thinking before it.
- **Stall detection is a heuristic.** A long build isn't a stall, but a six-hour `rm` is, and the line between them is a
  threshold picked by hand, not evidence in the file. Every stalled row carries the command and the duration so you can
  overrule it.
- **An outage is measured by the silence before it.** Claude Code records that a request failed, never that it retried,
  so the stretch before the error record is all there is to go on. That stretch is capped at two hours, and **the cap is
  a guess**: the longest such gap in the corpus this was built against is 1h19m, and nothing measures what the right
  ceiling would be. Past the cap the time is reported as idle instead.
- **Per-activity totals add up to more than the session took.** Agents run at the same time, so a three-day session can
  hold five days of agent time. Both numbers are reported separately, and the donut is a breakdown of agent time, never
  of the clock on the wall.
- **A tool execution span includes whatever the tool waited on**, a permission prompt included. Nothing in the
  transcript separates the two.

## Development

`pnpm check` runs every gate and says whether the repo is green. `AGENTS.md` is the doc hub: what's where, what the
conventions are, and which docs to read before changing what.

## License

Dual licensed under [MIT](LICENSE-MIT) and [Apache 2.0](LICENSE-APACHE), at your option.
