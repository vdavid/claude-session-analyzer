# Claude session analyzer

Reconstructs where the time went in a Claude Code session: per agent, second by second, from the first prompt to the
last tool result.

Scrolling a transcript tells you what happened. It doesn't tell you that a three-day multi-agent effort spent most of
its wall clock with the lead idle, or that one agent sat suspended for six hours on an `rm`. This reads the transcripts
on disk and answers that.

## Status

Early. The parse engine is in; the timeline, the CLI, the HTTP API, and the web UI are being built on top of it. There's
nothing to run yet.

## What it will do

- `claude-session-analyzer timeline <session-id>`: a CSV of every activity span, one row per content block.
- `claude-session-analyzer sessions`: what's on disk.
- `claude-session-analyzer serve`: a local web app with a time-spent pie, an agent-liveness swimlane, and a sortable
  data sheet.

## How it works

Claude Code writes one JSONL transcript per session under `~/.claude/projects/`, plus one per subagent it spawns. The
engine streams those, pairs tool calls with their results, and turns the record stream into labelled spans of thinking,
writing, tool calls, tool execution, waiting, and stalling.

Nothing is uploaded, stored, or cached. It reads the files you already have and holds the result in memory.

## Limitations

These are honest, not temporary:

- **Thinking content is unavailable.** Claude Code stores an empty string for reasoning text in almost every transcript,
  so a thinking span can only be labelled by what came after it.
- **Thinking spans include model latency.** A block's timestamp is when it finished streaming, so time in the API queue
  and in prompt processing lands inside the thinking span. On a large context that's a real share of it.
- **Stall detection is a heuristic.** A long build isn't a stall, but a six-hour `rm` is, and the line between them is
  drawn by a threshold, not by evidence in the file.

## License

Dual licensed under [MIT](LICENSE-MIT) and [Apache 2.0](LICENSE-APACHE), at your option.
