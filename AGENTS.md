# Claude session analyzer

Doc hub for AI agents working in this repo. People start at `README.md`.

Tool reads Claude Code session transcripts off disk, reconstructs where time went: per agent, second by second, first
prompt to last tool result. Answers "of these three days, how much was thinking, how much was tools running, how much
was the lead sitting idle waiting on a teammate?", "which tools did this session reach for, and which agents used
them?", and "did my agents prefer codegraph or grep this year?".

Three surfaces over one engine: a Go CLI (CSV timeline, JSON, and a `stats` query), a local web app (Svelte frontend,
same Go binary as backend) with time-spent pie, agent-liveness swimlane, tool-use pie, sortable data sheet, and
`skills/claude-session-stats/`, a Claude Code skill so agents reach for the CLI unprompted. That skill is the canonical
copy; `~/.claude/skills/claude-session-stats` is a symlink to it, so edit it here.

Personal tool, published because it's generic. Only transcripts are private. Dual licensed MIT and Apache-2.0.

## Where to look

- **Transcript format on disk**: `docs/transcript-format.md`. Read before touching the engine. Reverse-engineered and
  version-sensitive, cost real effort to establish: extend it rather than rediscovering it.
- **How a timeline is derived**: `docs/timeline-rules.md`. Every judgement call, with evidence. Read before changing a
  rule in `internal/timeline`, update it when you do.
- **What the HTTP API answers**: `docs/api.md`. Contract the web app is built against, and single source for the ladder
  of three durations a reader could quote as each other (lane time, net, active). Read before changing a JSON shape in
  `internal/report`.
- **What `stats` answers**: `docs/stats.md`. The query grammar, the dimensions, the three-clocks-per-call trap.
- **What's cached and how it goes stale**: `docs/cache.md`. Read before changing a stored shape or the key.
- **What the web app shows and why**: `docs/frontend.md`. Two pages, design system, decisions behind both. Editing
  must-knows in `web/CLAUDE.md` and colocated files under it.
- **Current build plan**: `docs/specs/initial-build-plan.md`. Plans get wiped periodically; durable knowledge belongs in
  `docs/`, not a plan.
- **Editing a package**: its colocated `CLAUDE.md` carries must-knows, harness injects it when you touch the directory.
  `internal/transcript/CLAUDE.md`, `internal/session/CLAUDE.md`, `internal/timeline/CLAUDE.md`,
  `internal/agg/CLAUDE.md`, `internal/report/CLAUDE.md`, `internal/cache/CLAUDE.md`, `internal/stats/CLAUDE.md`,
  `internal/cli/CLAUDE.md`, `internal/api/CLAUDE.md`, `internal/dotenv/CLAUDE.md`, and `web/CLAUDE.md` with three more
  under it.
- **Teaching an agent to use this tool**: `skills/claude-session-stats/SKILL.md`, the Claude Code skill. Keep its
  commands true: it's the one doc that gets read with no repo in front of it.
- **Finding a symbol**: Go doc comments say what each package owns, and `go doc ./internal/timeline` beats grep.

## Layout

- `cmd/claude-session-analyzer/`: the binary, nine lines over `internal/cli`.
- `internal/transcript/`: on-disk format. Record and block decoding, streaming line reader surviving megabyte-long
  lines. Retains nothing, so it works on the whole 3.8 GB corpus.
- `internal/session/`: session discovery under `~/.claude/projects`, `Lane` (lead plus one per subagent, each with its
  ordered records), `List` (summarizes every session by reading only the two ends of each transcript), and `Locations`
  (every session's files in one scan, because `Find` per session is quadratic).
- `internal/timeline/`: `Lane` records become activity rows. Activity kinds (`Kinds` lists them, four are waits), tool
  classification and naming (`Identify`, `Classes`), the seven tool categories the sixteen classes fold into
  (`category.go`, and it's configuration rather than engine truth), stall and timeout detection, waiting attribution.
  Rules in `docs/timeline-rules.md`.
- `internal/agg/`: one cube every total rolls up from. Sums a timeline once by lane, agent, kind, class, group, leaf,
  tool, and local day; `RollUp` keeps whichever dimensions a caller asks for. `ToolRuns`, `Composing`, and `Stalls` are
  the rule that a tool's own clock is neither the agent composing the call nor a call that stalled.
- `internal/report/`: JSON shapes both surfaces answer with, built from the cube. Contract in `docs/api.md`.
- `internal/cache/`: per-session digests on disk, so a corpus query doesn't reparse 3.8 GB. `docs/cache.md`.
- `internal/stats/`: filter, group, aggregate over digests. `docs/stats.md`.
- `internal/cli/`: `sessions`, `timeline`, `stats`, `cache`, and `serve`. `cli.Run(args, stdout, stderr) int` is the
  whole surface, so it's tested without a process.
- `internal/api/`: HTTP handlers over `internal/report`.
- `internal/dotenv/`: reads the committed `.env` holding dev ports.
- `web/`: SvelteKit frontend, a pnpm workspace beside the Go module. `src/lib/transform/` holds the tested layer (API
  JSON into chart series); `src/lib/components/charts/` holds one component per chart. `web/CLAUDE.md` first.
- `scripts/check.js`: the one command saying whether the repo is green. `scripts/docgraph/`: the doc-graph gate.
- `docs/`: this repo's docs. `docs/specs/`: plans.

## Docs

One tier, colocated. Each package and each meaningful frontend directory has a `CLAUDE.md` holding must-knows only:
gotchas, guardrails, short module map, pointer to the deep doc. Depth goes in `docs/`, not a `CLAUDE.md`. No
`DETAILS.md` tier here.

- **Single-source it.** A load-bearing claim lives in exactly one doc; everywhere else points at it by path. Copied
  prose rots on its own. Derivation rules belong to `docs/timeline-rules.md`, format's to `docs/transcript-format.md`,
  JSON contract's to `docs/api.md`, cache's to `docs/cache.md`, query grammar's to `docs/stats.md`.
- **Reference a doc as a bare backticked path** (`` `docs/api.md` ``), not a link repeating its own target. Link only
  for descriptive text or an anchor. `pnpm check docgraph` follows both, holds every doc to being reachable from this
  file with no dead paths.
- **Evidence-anchor a volatile claim**: a number, a version, or anything about the harness carries how and when it was
  checked. Transcript format drifts, and an undated confident claim about it becomes a landmine.

## Conventions

- Go for the engine, one binary for every surface. Parse on request; the only stored state is the digest cache, and
  `docs/cache.md` says what makes it valid.
- Engine holds the depth, where being wrong is invisible. UI stays thin.
- Tests table-driven against small hand-written fixtures in `testdata/`. No real transcript content beyond what a case
  needs, nothing personal.
- TDD where it matters: write failing test, watch it fail for the right reason, then implement.
- Every claim about the transcript format carries how and when it was verified. Format drifts.
- Ports live in a committed `.env` (nothing secret), bound to `127.0.0.1`.
- Output an agent reads is bounded by default. `stats` takes `--top`, `timeline --json` leaves rows out unless asked,
  `sessions` caps at 20. An 8 MB answer is a context-window incident.

## Writing voice

**Markdown docs here are caveman-compressed**, following the caveman-compress contract from
[caveman](https://github.com/JuliusBrussee/caveman): drop articles, filler, hedging, and connective fluff; fragments
fine; short synonyms. Agents read these every session, and the words around a rule cost tokens the rule doesn't.

What never compresses:

- Anything in a code fence or backticks, byte for byte: paths, commands, flags, type names, env vars.
- Heading text, list nesting, table structure.
- `not`, `never`, `no`, `only`, `except`. Flipping a meaning costs more than any token saved.
- The non-obvious why behind a rule, and its evidence anchor (dates, sample sizes, measurements). These docs exist so a
  future agent doesn't undo a decision it doesn't understand.
- Never invent abbreviations (`cfg`, `impl`, `req`): same token count, worse to read.

What stays normal prose: `README.md` (human front door), Go doc comments and code comments, and commit messages.
Caveman's own boundary rule says persisted-outside-chat prose stays normal; `caveman-compress` is the exemption, and it
covers markdown docs.

Also: sentence case in every title and label. Oxford comma. ISO dates. Spell out one through nine. No em dashes: use a
colon, a comma, parentheses, or a different sentence. Don't trivialize with "just", "simple", or "easy". Describe
current behavior, not how the code got there. Git holds the history.

## Running it

`pnpm dev` at the root starts both sides through `concurrently`: `go run ./cmd/claude-session-analyzer serve` and the
Vite dev server, on ports in the committed `.env`, both bound to `127.0.0.1`. One command, both logs, stopping it stops
both. Frontend reads the backend port from that same `.env`, so the two can't drift.

CLI:

```sh
go build -o claude-session-analyzer ./cmd/claude-session-analyzer

./claude-session-analyzer sessions --since 2026-07-01 --json
./claude-session-analyzer timeline 532ac591 --json
./claude-session-analyzer stats 532ac591 --where class=checker --group-by leaf
./claude-session-analyzer cache warm
```

## Checks

`pnpm check` at the root runs every gate, cheapest first, stops at first failure with that gate's full output:
`gofmt -l`, `go vet`, `docgraph`, `go test`, `prettier --check`, `eslint`, `svelte-check`, `vitest`. Scope it with an
argument, either a scope (`pnpm check go`, `web`, `docs`) or a gate name (`pnpm check vitest`).

`prettier` formats the whole repo from the root config, markdown included, so a doc reflows to 120 columns on
`pnpm format`. Don't hand-wrap prose against it.

CI (`.github/workflows/ci.yml`) runs that same `pnpm check` on every push to `main` and every pull request, with Go,
Node, and pnpm from `.mise.toml`, so a green laptop and a green run mean the same thing.

A green run is not evidence a page works. Drive the app against a real session before calling frontend work done.

Three tests read transcripts off this machine, so they skip by default. Run them after anything touching decoding,
because hand-written fixtures can't catch format drift:

- One session: `CSA_REAL_SESSION_ID=<id or unique prefix> go test ./internal/session -run RealSession -v`. Reports
  lanes, timings, every record type it skipped.
- Every session, listed the cheap way and cross-checked against a full parse:
  `CSA_REAL_LIST=1 go test ./internal/session -run RealListing -v`. About a second, catches a listing reading the wrong
  end of a file.
- Every transcript on the machine: `CSA_SWEEP=1 go test ./internal/session -run CorpusSweep -v -timeout 20m`. About 85 s
  over 4,460 transcripts. A record type in its skip list that isn't in `docs/transcript-format.md` means the format
  moved.

Derivation has three of its own, same idea. Run after changing a rule in `internal/timeline`:

- Where one session's time went: `CSA_REAL_SESSION_ID=<id> go test ./internal/timeline -run RealTimeline -v`.
- The two anomalies the tool exists for:
  `CSA_REAL_SESSION_ID=532ac591 go test ./internal/timeline -run RealAnomalies -v`.
- Every session on the machine: `CSA_SWEEP=1 go test ./internal/timeline -run RealTimelineSweep -v -timeout 30m`. About
  three minutes over 725 sessions, checks the tiling property on all of them, lists every stall it called.

Golden file (`internal/timeline/testdata/golden/timeline.csv`) holds the whole derivation to a committed CSV. Rewrite
with `go test ./internal/timeline -update`, read the diff before committing. Rewriting it also fails
`TestTheDigestVersionMovesWithTheDerivation` in `internal/cache`, on purpose: cached digests are answers under the old
rules, so `cache.Version` has to move with them.

Set `CSA_REAL_ROOT` to read from somewhere other than the default transcript root.
