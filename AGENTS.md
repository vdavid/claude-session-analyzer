# Claude session analyzer

This file is the doc hub for AI agents working in this repo.

The tool reads Claude Code session transcripts off disk and reconstructs where the time went: per agent, second by
second, from the first prompt to the last tool result. It answers questions like "of these three days, how much was
thinking, how much was tools running, and how much was the lead sitting idle waiting on a teammate?".

Two surfaces over one engine: a Go CLI that emits a CSV timeline, and a local web app (Svelte frontend, the same Go
binary as its backend) with a time-spent pie, an agent-liveness swimlane, and a sortable data sheet.

It's a personal tool, published because it's generic. Only the transcripts are private. Dual licensed MIT and
Apache-2.0.

## Where to look

- **The transcript format on disk**: `docs/transcript-format.md`. Read it before touching the engine. It's
  reverse-engineered and version-sensitive, and it cost real effort to establish, so extend it rather than
  rediscovering it.
- **The current build plan**: `docs/specs/initial-build-plan.md`. Plans get wiped periodically; durable knowledge
  belongs in `docs/`, not in a plan.
- **Package docs**: each `internal/` package has a doc comment saying what it owns.

## Layout

- `internal/transcript/`: the on-disk format. Record and block decoding, and a streaming line reader that survives
  megabyte-long lines. Retains nothing, so it works on the whole 3.8 GB corpus.
- `internal/session/`: session discovery under `~/.claude/projects`, and `Lane`, the lead plus one per subagent, each
  with its ordered records.
- `docs/`: this repo's docs. `docs/specs/`: plans.

## Conventions

- Go for the engine, one binary for both surfaces. No storage, no cache: parse on request, in memory.
- The engine holds the depth, where being wrong is invisible. The UI stays thin.
- Tests are table-driven against small hand-written fixtures in `testdata/`. No real transcript content beyond what a
  case needs, and nothing personal.
- TDD where it matters: write the failing test, watch it fail for the right reason, then implement.
- Every claim about the transcript format carries how and when it was verified. The format drifts.
- Ports live in a committed `.env` (nothing secret), bound to `127.0.0.1`.

## Writing voice

Sentence case in every title and label. Oxford comma. ISO dates. Spell out one through nine. Active voice, contractions
welcome. No em-dashes: use a colon, a comma, parentheses, or a different sentence. Don't trivialize with "just",
"simple", or "easy".

Docs here are for agents, not humans: prefer bullets to tables, and structure for retrieval. Describe current behavior,
not how the code got there. Git holds the history.

## Checks

`gofmt -l .`, `go vet ./...`, and `go test ./...` all have to be green. M6 wires a single `pnpm check` over these plus
the frontend gates.
