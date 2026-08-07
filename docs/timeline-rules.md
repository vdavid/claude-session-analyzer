# How the timeline is derived

What `internal/timeline` decides, and why. Parser's job: faithful to file. This package's job: honest about what the
file means, which takes judgement calls. Every one is here with its evidence, so it can be argued with rather than
believed.

Read `docs/transcript-format.md` first: the facts this builds on live there.

## The shape of the output

One row per stretch of one lane's wall clock, holding `From`, `Until`, `Agent`, `Activity`, `Extra info`, and a
duration. Engine hands back rows plus a span per lane; CLI writes the CSV, API aggregates.

**Rows tile their lane.** Each row starts where the last ended, first starts when the lane started, last ends when it
ended. Nothing unaccounted for, nothing counted twice. Only exception: a batch of parallel tool calls, which genuinely
overlap, and every row of one says so in `Overlapped`.

Holds across every session on the machine this was built against: 725 sessions, 4,340 lanes, 862,501 rows, no gaps and
no negative durations (verified 2026-08-06, `TestRealTimelineSweep`).

## The activity kinds

- **thinking**: a `thinking` block. Span starts when previous block finished streaming, so it includes API queue time
  and prompt processing, not only reasoning. On a 670k-token context that isn't noise, and it's why the legend says so
  rather than calling this "reasoning".
- **writing**: a `text` block, agent composing prose for its caller.
- **tool call**: composing the call, from previous block until `tool_use` block finished streaming. With no thinking
  block first, this span carries whole response latency, same caveat `thinking` has.
- **tool execution**: tool running, from `tool_use` block until its result came back. Honest wall clock of the tool,
  including whatever the tool itself waited on.
- **waiting for a person**: lane produced nothing until a person typed, queued a prompt, or answered a question the
  agent asked.
- **waiting for a teammate**: another agent's message closed the gap, or the notification of a background task that
  agent had left running while it was still alive.
- **waiting for a background task**: a background task's notification closed the gap, and the task wasn't a live
  teammate's.
- **waiting, reason unknown**: lane went quiet, later produced something, no input, message, or notification between.
- **API error**: API didn't answer the request. Span is harness retrying, time the session lost through no fault of the
  agent.
- **stalled**: result arrived far too late for what the call was doing. Agent was suspended, not working.
- **compacting**: harness compacting the context.

`Kind.IsWaiting()` is the four waits; `Kind.IsGap()` adds `stalled` and `API error`: everything where the lane produced
nothing, what a swimlane draws as a hole. `Kind.IsSomeoneElsesClock()` is the two waits whose time belongs to somebody
other than this lane's agent (a person's, a teammate's), which is the one thing net time subtracts from lane time:
`docs/api.md`. Ask those rather than listing kinds, so a new one can't be missed.

Two kinds the original plan didn't have. `compacting`, because compaction took 132 s in reference session and it's
neither agent thinking nor tool running, so folding it into a wait would call two minutes of real work idleness. And
`API error`, for the reason below.

### Why waiting is four kinds and not one

One bucket answers nothing. Reference session's lead sat idle 71h24m, and "71 hours of waiting" is not a finding: the
split is, because it says the session was blocked on its human far more than on its agents. Naming them for what was
waited on rather than who waited keeps a subagent's four minutes in the same buckets as the lead's 71 hours.

## The cursor, and why timestamps can't be trusted

Block's timestamp = when it **finished** streaming, so a span runs from previous record's timestamp to this one's.
Records go backwards, though: 186 of reference session's 15,831 do, mostly sub-millisecond write jitter, and compaction
replays a whole run of them two minutes behind.

So derivation walks a lane with a cursor that only moves forward. A record stamped before the cursor produces a
zero-length row rather than a negative one, and tiling holds by construction.

## Judgement calls

### A record with several blocks gives its span to the first one

Older transcripts pack a whole response into one record, up to 13 blocks including 11 parallel `tool_use` calls. They
share one timestamp, so only one can honestly claim the span that ended there. It goes to the first block, the thinking
block, where the time went. Rest are zero-length.

### Records that aren't work don't cost time

An attachment, a hook's output, a turn summary: they carry timestamps but aren't the lane doing something, so their
stretch belongs to whichever row surrounds them. Same for a block type nothing decodes, such as `fallback`.

### Telling an idle lane from a thinking one

The rule that goes wrong invisibly. A lane silent for hours that then produces a block reports those hours as
**thinking** unless something says otherwise, and a reader can't spot it.

Three signals, in order of how much they can be trusted:

1. **A turn ended.** `turn_duration`, `stop_hook_summary`, and `away_summary` all say the lane stopped. 46 of reference
   session's idle gaps over five minutes sit right after one.
2. **Input arrived.** A prompt, or a `queue-operation: enqueue`, timestamps the moment there was something to work on,
   where next turn's thinking starts. Queued input matters more than it looks: 41 of reference session's 78 long idle
   gaps end at an `enqueue` rather than a prompt, so watching only prompts misplaces half of them. An enqueue splits the
   lane whenever no tool call is open, without waiting for a turn-end record, because the harness doesn't always write
   one. Cost: input arriving while the agent is genuinely composing clips a few seconds off that row's front, bounded
   where the alternative isn't. One lane in corpus sat silent 7h23m after a text block, took a queued "Go on", answered
   three seconds later.
3. **The stretch is too long to be a response.** Backstop for lanes with neither signal, what a session resumed after
   `/exit` looks like: 25 days of nothing, then a text block. `DefaultMaxResponseSpan` is 15 minutes, and the constant
   carries the distribution that puts it there.

A stretch read as idle gets a waiting row, and the block that closed it claims none of it: nothing in the transcript
says when the lane actually woke up.

### A failed request is its own kind, and is bounded rather than trusted

Harness writes a failed request as an ordinary assistant record with a text block, so nothing but its typed fields
(`docs/transcript-format.md`) tells it from the agent writing that sentence. Read as prose it becomes a `writing` row;
read as a long silence it trips the 15-minute backstop above and becomes idle time. Either way an outage quietly lands
in a number that means something else, which is why it gets a kind.

`API error` covers everything the API refused, not only an outage: a rate limit, an expired login, a prompt too long, a
model that doesn't exist. Naming the kind "outage" would be wrong for half of them, and the row's info carries the typed
reason and the status, so the specific failure is right there.

Span is the stretch before the record, because retries aren't written down: no transcript in corpus holds two error
records in a row, so the gap before one is the only measure of the outage there is. Two clamps keep that honest, and
both leave a wait behind rather than swallowing time they can't account for:

- A lane whose turn had already ended had no request in flight, so the stretch before the error stays a wait and the
  error marks the moment without claiming any of it.
- Retry window can't run longer than `DefaultMaxAPIErrorSpan`, two hours against a corpus whose longest such gap is
  1h19m. The case it guards has no evidence and every reason to expect: a session resumed weeks later, whose first
  request fails on a login that expired while it sat there.

A lane is idle after a failed request, because the agent can't carry on by itself. Whatever comes next was started by
something else, and an unexplained gap after one says so.

### Compaction is placed by its own duration, not by the stamps around it

Boundary record is stamped when compaction finished, while records replayed after it keep older stamps. Measuring back
from the boundary with `compactMetadata.durationMs` puts the row where it belongs and leaves the idle stretch before it
as a wait. A boundary reporting no duration marks the moment and claims no time.

### Stall detection, and how hard it tries not to defame honest work

A stall says "the agent was suspended", an accusation. The line depends on what the call was doing:

- **An hour** for work with no plausible long form: `rm`, `ls`, `grep`, `git`, a file read, an MCP round trip.
- **Twelve hours** for work that earns its time: a build, a lint over a whole workspace, a test suite, a checker script,
  a dev server, a poll loop, or spawning a teammate.

Three kinds of call are never stalls whatever they cost. One the harness timed out was ended on purpose. One whose
result never arrived has no end to measure, because it was closed the moment we stopped reading. And a tool blocking
until a person answers (`AskUserQuestion`, `ExitPlanMode`) is **waiting**, not stalled: before that rule existed, three
of the five stalls in the whole corpus were questions a human took hours to answer.

What's left, over 862,501 rows: two stalls, both a trivial shell command back hours later (6h15m and 2h54m). Sweep
prints every one it finds, so a wrong call is visible rather than buried in a count.

Reading the command matters as much as the threshold. `until pgrep …; do sleep 3; done` is a poll loop and takes the
generous line; `pkill …; sleep 3; pgrep …` is not, because the sleep isn't what the command is for.

### Timeouts are read, then inferred

`timedOutAfterMs` settles it where the harness records it. Where it doesn't, a call that ran as long as it was allowed
and came back within a minute of that hit its limit; in reference session that second rule doubles what the first finds.
A call with no requested limit is left alone: harness default is two minutes, so inferring from it would put an ordinary
two-minute command on the line.

### Thinking rows borrow their subject, and say so

Thinking text is stripped from 5,469 of 5,471 sampled blocks. Where it survives the row quotes it; everywhere else the
row borrows from what the agent did next, always starting with "before", so a reader can tell a quote from a guess.

### A tool call is named twice, because the tools that matter are really many tools

A breakdown by raw tool name says almost nothing. Sampled 2026-08-06 over 76,708 calls in the 624 transcripts modified
since 2026-07-20: `Bash` is 47,711 of them (62%), `Edit` and `Read` take the top three to 92%, and the 1,769 MCP calls
arrive as one tool per method across 11 servers, so codegraph's 130 calls sit in four separate names, none of which is
"codegraph". So `Identify` gives every call two names, and rows carry both.

- **Group** is the level a reader asks about. `Bash` splits by the class its command was read as, giving `Bash (git)`
  and `Bash (checker)`; an MCP server's methods collapse into `codegraph (MCP)`; everything else is its own name. MCP
  separator is two underscores, so a server or method carrying single ones (`claude_ai_Gmail`, `get_thread`) comes
  through whole.
- **Leaf** is the exact thing that ran. For an MCP call, the method. For `Bash`, the program of the segment that named
  the command, carrying the subcommand wherever the subcommand is the whole story: `git commit` and `git status` are as
  different as two tools, and so are `cargo build` and `cargo test`. A program invoked by path is named by its file, so
  a gate reached through the directory it lives in is `check.sh` rather than the path to it.

Both names come out of one read of the command, which is also where its class comes from. A compound command is named
after the costliest thing in it, so `git add -A && pnpm check` is a checker run and `curl … | jq` is a fetch: a network
round trip explains a command's time better than reading what it fetched does. Every class a command can be read as has
to be in that precedence list, or it can never outrank the plain shell command every command starts out as.

### A class says what the work was for, not what it mechanically is

`cargo check` compiles. Read by mechanism it's a build; read by purpose it's a verification that produces nothing, and
the breakdown answers "what was the agent doing". Purpose wins, so the classes split on what a call leaves behind:

- **`build`** produces an artifact: `cargo build`, `cargo install`, `go build`, `go generate`, `pnpm build`,
  `tsc --build`, `webpack`, `esbuild`, `xcodebuild`, `gcc`, `javac`, `gradle`, `mvn`. `cargo doc` is here too: HTML you
  can open is an artifact.
- **`lint`** verifies and leaves nothing: `cargo check`, `cargo clippy`, `cargo fmt`, `go vet`, `gofmt`, bare `tsc`,
  `prettier`, `eslint`, `svelte-check`, `ruff`, `golangci-lint`.
- **`checker`** is the project's own umbrella gate, `pnpm check` or `make check` or a `check.sh` reached by path, and
  stays separate from the linters running inside it. "The gate took 110 hours over 7,774 runs" and "clippy took 49
  minutes of it" are different questions, and folding one into the other loses both. A runner script called `lint`,
  `typecheck`, or `format` is a project's own gate by the same convention as `check`.

A name alone still can't be trusted, which is why these need a rule at all: `cargo check` and `pnpm check` share a word
and are different animals, and `tsc` is one program doing both jobs (`--build` or `-b` emits, anything else typechecks).
Where a subcommand means different work in different toolchains, the pair decides: `cargo doc` renders HTML, `go doc`
prints a package's comments to the terminal, so that one is a file read.

`lint` sits under `build` and `test` in the precedence list and over `git`, so `cargo clippy && cargo build` is a build,
`cargo fmt && cargo test` is a test run, and `cargo fmt && git add -A` is a lint. It takes the generous stall line
(below) beside them, because a clippy run over a workspace earns its minutes the same way a build does.

Measured over the whole corpus 2026-08-08, 736 sessions: the split moved 2,028 calls and 9h56m out of `build`, leaving
1,433 calls and 5h42m there, and picked up another 2h47m of formatters and linters that had been reading as `shell`, a
pipeline's `grep`, or a `wc -l`. `lint` lands at 2,558 calls and 12h01m across 182 sessions. `test` grew 1h16m, which is
the honest half of the same change: a `cargo fmt && cargo test` had been named after its lint prefix.

Managing the harness is `agent` work rather than the thing it resembles: `ToolSearch` searches for tools, not for code,
and `EnterWorktree`, `ExitWorktree`, and `TaskStop` are how a teammate is put to work and stopped. `Skill` stays
unrecognised, because what a skill did is whatever it ran, and nothing in the call says.

Two things a leaf has to see through, both found by reading reference session's own output:

- **Arranging the shell isn't work.** `cd`, `export`, `source`, and friends get no vote on what a command was, so
  `cd apps/desktop && python3 tool.py` is a Python run. Before that rule, `cd` named 212 of that session's 7,057 calls.
  A command that only arranges the shell still gets named after what it did.
- **`timeout` hides the command behind a duration.** Stripped along with duration and any flags, same as `sudo` and
  `nohup`, so `timeout 120 cargo test` is a test run. It had been hiding a 21-minute `cargo nextest` and a
  `npx playwright` run inside "shell".

### A wait is attributed by the record that ended it

The record that arrived is the signal: a teammate's message (read from the harness's envelope, which carries sender's
id), a background task's notification, or a person's prompt. A wait nothing arrived to end is `waiting, reason unknown`
rather than a guess.

Leaves no ambiguity to resolve: across the corpus's 12,437 envelope-carrying records, not one carries both envelopes
(verified 2026-08-06). A notification arrives as a plain prompt about a third of the time (2,044 against 6,288 queued),
so both paths are read the same way.

Envelope is only looked for in the first 200 characters, the message's own wrapper rather than something it quotes.
Three enqueued messages in corpus carry a notification past that point and are read as a person's prompt, the deliberate
side of that trade.

Lead waits also list teammates alive at the time, the difference between "blocked on four agents" and "blocked on
nobody". A sweep over lanes sorted by start with a heap of the ones still running, because one session in corpus has 977
workflow lanes and a scan per row would not do.

A notification is the one signal that gets a second question asked, because it says a task finished, not that the lane
was waiting for the task. Session `532ac591`, measured 2026-08-07: 14 of lead's waits read as waiting on the dev app,
5.97 h of lead idle against 3.19 h of task, six of the tasks under a second. Worst one, lead idle 25m30s, woken by

```sh
until grep -q "Running DevCommand\|app started\|Ready in" .../dev.log; do sleep 3; done; echo "app launched"
```

which subagent `m1-honesty` started at 08:04:47.767 and which finished at 08:04:47.908. **0.14 s.** Lead was waiting for
`m1-honesty`, alive and working throughout; reading that as a background task's wait claims the app took 25 minutes to
launch, and reads as true.

So a notification-closed gap is `waiting for a teammate` when both of these hold, and `waiting for a background task`
otherwise:

1. notification's `<tool-use-id>` belongs to a lane other than the waiting one, and
2. that lane was still alive when the gap ended (`LaneSpan.First <= row.Until <= LaneSpan.Last`).

Condition 2 is not droppable: a subagent that starts a 40-minute build, reports back, and finishes leaves the lead
genuinely waiting on the build, which is a real background task's wait. Only the still-running case is the lead blocked
on the teammate. Info reads `waiting for teammate <name>, via its background task`: same phrasing as every other
teammate wait, plus how it was established.

Ownership comes from a pre-pass mapping every `tool_use` id in every lane to its lane, every lane rather than the one
being walked, because the owner is nearly always a different one. An id no lane claims keeps
`waiting for a background task`, which covers an owner's transcript compacted away, and a notification carrying no
`<tool-use-id>` at all: 9,884 of the corpus's 12,076 notifications carry one, and 1,726 of the rest are `Monitor` events
(verified 2026-08-07). Unknown means unchanged, never a guess.

## What this doesn't do

- Can't say when a suspended agent woke up, only that the result arrived late. Same for a lane resuming with no input on
  record: the row before it is idle, and the block that ended it claims no time.
- Can't split a multi-block record's span across its blocks: only one timestamp.
- A thinking span includes model latency and always will, and so does a tool call span with no thinking block before it.
- A tool execution span includes anything the tool waited on, including a person answering a permission prompt. Nothing
  in the transcript separates the two.
- Two lanes can carry the same name, because a lane with no `.meta.json` falls back to its agent type. Group by
  `LaneID`, not by `Agent`.
- Stall detection is a heuristic. Every stalled row carries the command, the duration, and the line it was measured
  against, so a reader can overrule it.
