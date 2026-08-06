# Claude session analyzer

This file is the doc hub for AI agents working in this repo. People start at `README.md`.

The tool reads Claude Code session transcripts off disk and reconstructs where the time went: per agent, second by
second, from the first prompt to the last tool result. It answers questions like "of these three days, how much was
thinking, how much was tools running, and how much was the lead sitting idle waiting on a teammate?".

Two surfaces over one engine: a Go CLI that emits a CSV timeline, and a local web app (Svelte frontend, the same Go
binary as its backend) with a time-spent pie, an agent-liveness swimlane, and a sortable data sheet.

It's a personal tool, published because it's generic. Only the transcripts are private. Dual licensed MIT and
Apache-2.0.

## Where to look

- **The transcript format on disk**: `docs/transcript-format.md`. Read it before touching the engine. It's
  reverse-engineered and version-sensitive, and it cost real effort to establish, so extend it rather than rediscovering
  it.
- **How a timeline is derived**: `docs/timeline-rules.md`. Every judgement call the derivation makes, with the evidence
  behind it. Read it before changing a rule in `internal/timeline`, and update it when you do.
- **What the HTTP API answers**: `docs/api.md`. The contract the web app is built against, including the two totals a
  reader could confuse. Read it before changing a JSON shape in `internal/api`.
- **What the web app shows and why**: `docs/frontend.md`. The two pages, the design system, and the decisions behind
  both. Editing must-knows are in `web/CLAUDE.md` and the colocated files under it.
- **The current build plan**: `docs/specs/initial-build-plan.md`. Plans get wiped periodically; durable knowledge
  belongs in `docs/`, not in a plan.
- **Editing a package**: its colocated `CLAUDE.md` carries the must-knows, and the harness injects it when you touch the
  directory. `internal/transcript/CLAUDE.md`, `internal/session/CLAUDE.md`, `internal/timeline/CLAUDE.md`,
  `internal/cli/CLAUDE.md`, `internal/api/CLAUDE.md`, `internal/dotenv/CLAUDE.md`, and `web/CLAUDE.md` with three more
  under it.
- **Finding a symbol**: the Go doc comments say what each package owns, and `go doc ./internal/timeline` beats grep.

## Layout

- `cmd/claude-session-analyzer/`: the binary, nine lines over `internal/cli`.
- `internal/transcript/`: the on-disk format. Record and block decoding, and a streaming line reader that survives
  megabyte-long lines. Retains nothing, so it works on the whole 3.8 GB corpus.
- `internal/session/`: session discovery under `~/.claude/projects`, `Lane` (the lead plus one per subagent, each with
  its ordered records), and `List`, which summarizes every session on disk by reading only the two ends of each
  transcript.
- `internal/timeline/`: `Lane` records become activity rows. The activity kinds (`Kinds` lists them, four of them
  waits), tool classification, stall and timeout detection, and waiting attribution. Rules in `docs/timeline-rules.md`.
- `internal/cli/`: the `sessions`, `timeline`, and `serve` subcommands. `cli.Run(args, stdout, stderr) int` is the whole
  surface, so it's tested without a process.
- `internal/api/`: the HTTP handlers and the JSON shapes. Contract in `docs/api.md`.
- `internal/dotenv/`: reads the committed `.env` that holds the dev ports.
- `web/`: the SvelteKit frontend, a pnpm workspace beside the Go module. `src/lib/transform/` holds the tested layer
  (API JSON into chart series); `src/lib/components/charts/` holds one component per chart. `web/CLAUDE.md` first.
- `scripts/check.js`: the one command that says whether the repo is green. `scripts/docgraph/`: the doc-graph gate.
- `docs/`: this repo's docs. `docs/specs/`: plans.

## Docs

One tier, colocated. Each package and each meaningful frontend directory has a `CLAUDE.md` holding must-knows only:
gotchas, guardrails, a short module map, and a pointer to the deep doc. Depth goes in `docs/`, not in a `CLAUDE.md`, and
there's no `DETAILS.md` tier here.

- **Single-source it.** A load-bearing claim lives in exactly one doc, and everywhere else points at it by path. Copied
  prose rots on its own. The derivation's rules belong to `docs/timeline-rules.md`, the format's to
  `docs/transcript-format.md`, and the JSON contract's to `docs/api.md`.
- **Reference a doc as a bare backticked path** (`` `docs/api.md` ``), not as a link repeating its own target. Link only
  for descriptive text or an anchor. `pnpm check docgraph` follows both, and holds every doc to being reachable from
  this file with no dead paths.
- **Evidence-anchor a volatile claim**: a number, a version, or anything about the harness carries how and when it was
  checked. The transcript format drifts, and an undated confident claim about it becomes a landmine.

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

## Running it

`pnpm dev` at the root starts both sides through `concurrently`: `go run ./cmd/claude-session-analyzer serve` and the
Vite dev server, on the ports in the committed `.env`, both bound to `127.0.0.1`. One command, both logs, and stopping
it stops both. The frontend reads the backend port from that same `.env`, so the two can't drift.

## Checks

`pnpm check` at the root runs every gate, cheapest first, and stops at the first failure with that gate's full output:
`gofmt -l`, `go vet`, `docgraph`, `go test`, `prettier --check`, `eslint`, `svelte-check`, and `vitest`. Scope it with
an argument, either a scope (`pnpm check go`, `web`, `docs`) or a gate name (`pnpm check vitest`).

`prettier` formats the whole repo from the root config, markdown included, so a doc reflows to 120 columns on
`pnpm format`. Don't hand-wrap prose against it.

A green run is not evidence a page works. Drive the app against a real session before calling frontend work done.

Three more tests read transcripts off this machine, so they skip by default. Run them after anything that touches
decoding, because hand-written fixtures can't catch format drift:

- One session: `CSA_REAL_SESSION_ID=<id or unique prefix> go test ./internal/session -run RealSession -v`. Reports
  lanes, timings, and every record type it skipped.
- Every session, listed the cheap way and cross-checked against a full parse:
  `CSA_REAL_LIST=1 go test ./internal/session -run RealListing -v`. Takes about a second, and catches a listing that
  reads the wrong end of a file.
- Every transcript on the machine: `CSA_SWEEP=1 go test ./internal/session -run CorpusSweep -v -timeout 20m`. Takes
  about 37 s over 4,452 transcripts. A record type appearing in its skip list that isn't in `docs/transcript-format.md`
  means the format moved.

The derivation has three of its own, same idea. Run them after changing a rule in `internal/timeline`:

- Where one session's time went: `CSA_REAL_SESSION_ID=<id> go test ./internal/timeline -run RealTimeline -v`.
- The two anomalies the tool exists for:
  `CSA_REAL_SESSION_ID=532ac591 go test ./internal/timeline -run RealAnomalies -v`.
- Every session on the machine: `CSA_SWEEP=1 go test ./internal/timeline -run RealTimelineSweep -v -timeout 30m`. Takes
  about a minute over 724 sessions, checks the tiling property on all of them, and lists every stall it called.

The golden file (`internal/timeline/testdata/golden/timeline.csv`) holds the whole derivation to a committed CSV.
Rewrite it with `go test ./internal/timeline -update` and read the diff before committing it.

Set `CSA_REAL_ROOT` to read from somewhere other than the default transcript root.
