# Claude session analyzer

Reconstructs where the time went in a Claude Code session: per agent, second by second, from the first prompt to the
last tool result.

Scrolling a transcript tells you what happened. It doesn't tell you that a three-day multi-agent effort spent most of
its wall clock with the lead idle, or that one agent sat suspended for six hours on an `rm`. This reads the transcripts
on disk and answers that.

## Status

Early, and it works. The engine, the command line, the HTTP API, and the web app are in.

## Quick start

The web app, both halves at once:

```sh
pnpm install
pnpm dev            # the Go server and the frontend, on the ports in `.env`
```

Then open http://127.0.0.1:19428. The front page lists every session on the machine; opening one shows where its time
went. Both services bind to `127.0.0.1` and nothing else: a tool that reads every transcript on your machine has no
business on the network.

The command line, if you'd rather have a CSV:

```sh
go build -o claude-session-analyzer ./cmd/claude-session-analyzer

./claude-session-analyzer sessions                 # what's on disk, newest first
./claude-session-analyzer timeline 532ac591        # the CSV, to standard output
./claude-session-analyzer timeline 532ac591 --out timeline.csv
./claude-session-analyzer serve                    # the API alone, on http://127.0.0.1:19427
```

A session id can be any prefix that matches one session, which is why `532ac591` works. `sessions` lists 722 sessions
and 3.5 GB in a quarter of a second, because it reads the two ends of each transcript rather than any of the middle.

## The CSV

One row per stretch of one agent's wall clock, in six columns: `From`, `Until`, `Agent`, `Activity`, `Extra info`, and
`Duration (s)`. `Activity` is one of thinking, writing, tool call, tool execution, waiting for a person, waiting for a
teammate, waiting for a background task, waiting with the reason unknown, API error, stalled, or compacting. Waiting is
four values rather than one because "71 hours of waiting" answers nothing, while knowing that 41 of those hours were on
a person does. Within an agent the rows tile: each starts where the last ended, so nothing is unaccounted for and
nothing is counted twice. What each activity means, and every judgement call behind it, is in `docs/timeline-rules.md`.

## What the web app shows

Open a session and it derives the timeline on the spot, then shows four things:

- **A trace of how many agents were producing at once**, across the whole span. It's the shape of the session in one
  line, including the stretches where nobody was working.
- **Elapsed time beside lane time.** Those are different numbers and the page never mixes them up: lane time adds every
  agent's clock together, so two agents working side by side for an hour is two hours of lane time and one hour of
  elapsed time.
- **A donut of lane time by activity**, with a legend that spells out what each slice does and doesn't include.
- **A swimlane of who was alive when.** A thick bar means the agent was producing something; a thin one means it wasn't,
  coloured by what it was waiting on. A session with a thousand agents collapses its workflows into a row each, opening
  on click.
- **Every derived row**, sortable and filterable, down to sub-second spans.

## How it works

Claude Code writes one JSONL transcript per session under `~/.claude/projects/`, plus one per subagent it spawns. The
engine streams those, pairs tool calls with their results, and turns the record stream into labelled spans of thinking,
writing, tool calls, tool execution, waiting, and stalling. A wait is attributed to whatever ended it: a person, a
teammate, or a background task.

Nothing is uploaded, stored, or cached. It reads the files you already have and holds the result in memory.

## Limitations

These are honest, not temporary:

- **Thinking content is unavailable.** Claude Code stores an empty string for reasoning text in almost every transcript,
  so a thinking span can only be labelled by what came after it.
- **Thinking spans include model latency.** A block's timestamp is when it finished streaming, so time in the API queue
  and in prompt processing lands inside the thinking span. On a large context that's a real share of it.
- **Stall detection is a heuristic.** A long build isn't a stall, but a six-hour `rm` is, and the line between them is
  drawn by a threshold, not by evidence in the file.
- **An outage is measured by the silence before it.** Claude Code records that a request failed, never that it retried,
  so the stretch before the error record is all there is to go on. It's capped at two hours, past which the time is
  reported as idle instead.
- **Time per activity adds up to more than the session took.** Agents run at the same time, so a three-day session can
  hold five days of agent time. That's the honest answer, and both numbers are reported separately.

## License

Dual licensed under [MIT](LICENSE-MIT) and [Apache 2.0](LICENSE-APACHE), at your option.
