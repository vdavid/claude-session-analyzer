# Initial build plan

Status: IN EXECUTION

## What we're building

A tool that reads Claude Code session transcripts off disk and reconstructs **where the time went**: per agent, second by
second, from the first prompt to the last tool result.

The question that started it: a large multi-agent effort took three days of wall clock. Which parts were thinking, which
were tool execution, which were the lead sitting idle waiting on a teammate, and which were neither (a suspended agent,
a timed-out wait loop)? Nobody can answer that by scrolling a transcript.

Two surfaces over one engine:

1. A Go CLI that emits a CSV timeline for any session id.
2. A local web app (Svelte frontend, the same Go binary as its backend) that lists sessions, and for one session shows a
   time-spent pie, an agent-liveness swimlane, and the full sortable and filterable data sheet.

This is a personal tool for David, published because it is generic. Keep it lean. Depth belongs in the engine, where
being wrong is invisible; the UI can stay thin.

## What the data looks like

All of this is verified against a real 62 MB session (`532ac591-b7c5-45ca-a764-f40f01a0a9ac`, 25 lanes, 2026-08-03 to
2026-08-06). Do not re-derive it, but do assert it: a fixture-backed test per claim is cheap, and the format will drift.

### Layout on disk

- Sessions live at `~/.claude/projects/<project-slug>/<session-id>.jsonl`. The slug is the project path with `/` and `.`
  replaced by `-`, so it is lossy; don't try to invert it, read `cwd` off any record instead.
- A session that spawned subagents also has a sibling directory `~/.claude/projects/<project-slug>/<session-id>/`
  containing `subagents/agent-a<name>-<hash>.jsonl` plus a `.meta.json` per agent, and sometimes `tool-results/` holding
  offloaded large tool outputs.
- `.meta.json` carries `agentType`, `name`, `description`, `model`, `spawnDepth`, `taskKind`, `color`. Use `name` as the
  lane label and `color` for the swimlane, so the chart matches what the terminal showed.
- Sibling `.jsonl.wakatime` files are unrelated. Ignore them.
- Scale to design for: 4,435 transcripts, 3.8 GB total on this machine. The session list must not parse full bodies.

### Record shape

One JSON object per line. Not every line is a message: `custom-title`, `agent-name`, `mode`, `permission-mode`,
`queue-operation`, `worktree-state`, `file-history-snapshot`, and others appear too. Unknown `type` values must be
skipped without failing; new ones get added over time.

The types that matter:

- `assistant`: one record **per content block**, not per message. A single API response becomes a `thinking` record, then
  a `text` record, then a `tool_use` record, each separately stamped. `requestId` groups them; `message.usage` carries
  token counts (on the final block of the request, earlier blocks hold partial counts).
- `user`: either a real prompt (`message.content` is a string) or a tool result (`message.content` is an array holding a
  `tool_result` block, with `toolUseResult` alongside carrying the structured payload).
- `attachment`: hook output, task reminders, memory injections, file edits. Carries `attachment.type`. Not agent work;
  the timeline should not spend time on these, but the parser must not trip over them.
- `system`: `subtype` of `turn_duration` (has `durationMs` and `messageCount`), `away_summary`, `stop_hook_summary`,
  `compact_boundary`, `local_command`.
- `queue-operation`: `enqueue` (with `content`, the user's queued text) and `dequeue`. **This is how you spot a prompt
  that arrived while the agent was busy**, and it timestamps when the user actually typed, not when it was consumed.

### Timing semantics

- A block's `timestamp` is when the block **finished streaming**, not when it started. So a span runs from the previous
  record's timestamp to this record's timestamp.
- Therefore a `thinking` span unavoidably includes API queue time and prompt processing. On a 670k-token context that is
  not noise. Label it honestly (see Decision 4); do not present it as pure reasoning time.
- Tool execution is exact: `tool_use` timestamp to the matching `tool_result` timestamp, paired on `tool_use_id`.
- **`thinking.thinking` is always the empty string.** Only `signature` survives, so thinking content is unavailable.
  There is no way around this; don't spend time looking.
- The verified session has **zero multi-tool requests**: no assistant response ever emitted more than one `tool_use`.
  Parallel tool calls are possible in general, so handle them (Decision 5), but they are not the common case.

### Anomalies the engine must name

Both found in the verified session, both invisible without this tool:

- **Timed-out waits.** Repeated 600.0s tool results from blocking shell loops (`until pgrep …; do sleep 3; done`) hitting
  the 10-minute Bash cap. The agent was neither thinking nor working.
- **Suspension.** A trivial `rm` plus `du -sh` whose result arrived **6h20m** after it was issued. The agent was stalled,
  not busy. Reporting that as a six-hour `rm` would be a lie, and it is exactly the event that made the lead believe a
  teammate had died.

## Decisions

1. **Go for the engine, one binary serving both surfaces.** 62 MB for one session, 3.8 GB across all of them, and the
   cross-session stats David wants next are a streaming problem. Go's line-oriented decoding holds a constant heap. The
   static binary also means `pnpm dev` needs no compile step for the backend beyond `go run`.
2. **No storage, no cache.** Parse on request, in memory. A session parses in well under a second; a cache would be
   another thing to invalidate when a transcript grows mid-session.
3. **One row per content block**, not per message. David asked for the fine grain explicitly. Expect ~15k rows for the
   verified session, which the data sheet must handle without choking.
4. **Six activity kinds**, and the naming is load-bearing:
   - `thinking` (includes model latency, say so in the docs and in the UI legend)
   - `writing` (a `text` block: the agent composing prose for its caller)
   - `tool call` (composing the call: previous block to the `tool_use` block)
   - `tool execution` (`tool_use` to `tool_result`, the honest wall clock of the tool)
   - `waiting` (an idle gap; the lead waiting on a teammate or on David)
   - `stalled` (a tool result that arrived absurdly late: the agent was suspended, not working)
5. **Parallel tool calls stay separate rows and are allowed to overlap.** David asked for strictly sequential lanes, and
   in practice they are, but silently merging concurrent calls would hide real concurrency. Emit each, and flag the
   overlap in `Extra info` so a reader knows the lane briefly forked.
6. **Sub-second rows are kept.** David asked for this. It means the pie must aggregate by duration, not by row count.
7. **Thinking rows borrow their subject from what follows.** With thinking content unavailable, the only honest label is
   the tool call or text that came next: `before Bash: cargo test coverage::`. Phrase it so it reads as inference.
8. **Waiting rows name what ended them.** A `queue-operation: enqueue` carrying user text means the agent was waiting on
   David; a teammate message means it was waiting on a subagent. Annotate lead waiting rows with which subagents were
   alive at that moment, computed from the subagent lane spans. This is the single most useful column in the file.
9. **Public repo, dual MIT / Apache-2.0**, matching `gitstrata`. The tool is generic; only the transcripts are private.
   No real transcript content in fixtures beyond what is needed, and nothing personal.
10. **Ports live in a committed `.env`.** No secrets are involved, and a fresh clone should run `pnpm dev` and work.
    Backend `19427`, frontend `19428`, both bound to `127.0.0.1`.

## Milestones

Run these in order. Each ends green and committed.

### M1: Repo skeleton and the parse engine

Scaffold: `go.mod`, `.mise.toml` (node 26, pnpm 11.18.0, go 1.26.5), `.gitignore`, `.editorconfig`, both LICENSE files
(already present), `AGENTS.md` + root `CLAUDE.md` containing `@AGENTS.md`, and a first `README.md`.

The engine, under `internal/`:

- Session discovery: given a bare session id, find it under `~/.claude/projects/*/`, and locate its subagent lanes.
  Return a clear message when the id is unknown or ambiguous.
- A streaming JSONL reader that decodes the record types above and skips unknown ones without erroring. Lines can be
  large; size the scanner buffer accordingly and prove it with a fixture.
- A `Lane` type: the lead plus one per subagent, each with a name, model, colour, and its ordered records.

Tests: table-driven, against small fixtures committed under `testdata/`. Cover the record types, an unknown type, an
oversized line, a missing session, and a session with no subagents.

### M2: Timeline derivation

Turn lanes into activity rows. This is where the thinking goes, and where being wrong is invisible, so this milestone is
test-first (`tdd-red-green.md`): write the failing case, watch it fail for the right reason, then implement.

- The six activity kinds from Decision 4, with the span rules from § Timing semantics.
- Tool classification from the `tool_use` input, so the CSV can say what kind of work a `Bash` call was. At minimum:
  checker script, build, test, git, file read/write, search, dev server, MCP, and a plain fallback. Read the command
  string; do not guess from the description alone.
- Stall detection per Decision 4. Pick a threshold that does not defame a legitimately long build, and document the
  reasoning next to the constant. A long `cargo build` is not a stall; a 6-hour `rm` is.
- Waiting attribution per Decision 8.
- Row output: `From`, `Until`, `Agent`, `Activity`, `Extra info` in that order, plus a duration column (David listed
  five, but a duration column costs nothing and every consumer wants it; put it last so his five stay first).

Tests: the derivation rules, each anomaly kind, an agent that never idles, a lane with a parallel tool batch, and a
golden-file test over a trimmed real session so a rule change shows up as a reviewable diff.

### M3: CLI

`claude-session-analyzer timeline <session-id> [--out file.csv]` writing the CSV, and `sessions` listing what is on
disk. Subcommand shape from the start so `stats` and `tokens` can land later without a rewrite. Use the standard library
flag package; this does not need a CLI framework.

The `sessions` listing must be cheap: it reads file sizes, mtimes, the first and last record of each transcript, and the
subagent directory count. It must not parse 3.8 GB of bodies.

Tests: golden CSV output, and a listing test that asserts the cheap path is taken.

### M4: HTTP API

The same binary, `claude-session-analyzer serve --port …`, reading the port from `.env`, bound to `127.0.0.1`.

- `GET /api/sessions`: id, project path, title, start, end, wall-clock duration, subagent count, size on disk. Sorted
  newest first.
- `GET /api/sessions/{id}/timeline`: the rows, plus pre-aggregated pie totals (duration per activity kind per agent) and
  swimlane spans (per agent: first activity, last activity, and the idle gaps inside). Aggregate server-side. Shipping
  15k rows and making the browser sum them is the wrong split for a Go backend.
- `GET /api/sessions/{id}`: the metadata for one session, for the session page header.

Return real HTTP status codes and a JSON error body. Tests: handler tests over the fixture sessions.

### M5: Frontend

SvelteKit with `adapter-static`, Svelte 5 runes, Tailwind v4, mirroring `gitstrata`'s setup (that repo is the reference
for config, lint, and format; copy its shape, not its content).

Two routes:

- `/`: what this is, in a few honest sentences, plus the session list. Each row shows title, project, when, wall-clock
  length, and subagent count, and links to the session page.
- `/session/[id]`: analyses on load, then shows the pie, the swimlane, and the data sheet.

Charts with **ECharts**: a pie for time spent, and a horizontal floating-bar swimlane with `dataZoom` so a 71-hour span
can be scrubbed. Data sheet with **TanStack Table** for sorting and filtering. Do not hand-roll either.

Verify the latest versions of every dependency on npm before pinning (never from memory), and respect the three-day
release-age window. Run `pnpm dedupe` after installing.

Design bar: this should look considered, not bootstrapped. Dark and light both real, respecting
`prefers-color-scheme`. Sentence case everywhere. Colour the swimlane lanes from each agent's recorded `color` so the
chart matches the terminal. David will ask for more charts later, so keep the chart components separable rather than one
god component.

### M6: Wiring, checks, docs, and publish

- `pnpm dev` starts both, via `concurrently`, on the `.env` ports. One command, both logs, and killing it kills both.
- A `pnpm check` that runs the Go and the frontend gates: `gofmt -l`, `go vet`, `go test`, `svelte-check`, `eslint`,
  `prettier --check`, `vitest`. Lean, fast, and it must be the single command that says whether the repo is green.
- `README.md`: what it is, what it answers, a screenshot-free quick start, the CSV column reference, and an honest
  limitations section (thinking content unavailable, thinking spans include model latency, stall detection is a
  heuristic).
- `AGENTS.md` as the hub per David's convention, with the transcript-format knowledge from § What the data looks like
  moved into a doc the engine points at, so the next agent doesn't rediscover it.
- Create the public GitHub repo under `vdavid` and add the remote. **Do not push**; David pushes on his own schedule.

## Sequencing

M1 → M2 → M3 → M4 → M5 → M6, strictly. M3 and M4 both sit on M2's output and could overlap, but they are small and the
coordination cost is not worth it.

## Execution status

**M1 done.** Repo skeleton, `internal/transcript` (streaming reader), and `internal/session` (discovery and lanes).
Verified against every transcript on the machine: 4,438 files, 1,221,828 lines, 0 malformed.

Four things the plan above got wrong or missed, all now corrected in `docs/transcript-format.md`, which is the doc to
trust from here:

1. **Workflow subagents are lanes and the plan doesn't mention them.** They live one level deeper, at
   `subagents/workflows/wf_<id>/agent-*.jsonl`. One session in the corpus has 977 of them. M5 needs a plan for
   drawing a session with a thousand lanes.
2. **"One record per content block" is version-dependent.** Newer transcripts split them, older ones pack up to 13
   blocks into one record, including 11 parallel `tool_use` calls. Decision 5 (parallel calls stay separate rows) is
   therefore load-bearing, not theoretical.
3. **`thinking.thinking` is not always empty.** It's empty in 5,469 of 5,471 sampled blocks, but two on `2.1.177`
   carry real reasoning. Decision 7 should use the text when it's there and fall back to inference otherwise.
4. **Timestamps go backwards.** 186 of 15,831 records in the reference session, mostly sub-millisecond jitter, but
   compaction produces a real 132 s backward jump. M2 must clamp negative spans and read compaction time from
   `compactMetadata.durationMs`.

Also worth knowing for M2: subagent `.meta.json` is often missing or partial, so `Lane.Name` already falls back
through `name` → `agentType` → agent id, and `Meta.Color` is empty often enough that the UI needs its own palette.
