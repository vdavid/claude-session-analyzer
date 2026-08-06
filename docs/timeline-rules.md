# How the timeline is derived

What `internal/timeline` decides, and why. The parser's job is to be faithful to the file; this package's job is to be
honest about what the file means, which involves judgement calls. Every one of them is here, with the evidence behind
it, so it can be argued with rather than believed.

Read `docs/transcript-format.md` first: the facts this builds on live there.

## The shape of the output

One row per stretch of one lane's wall clock, holding `From`, `Until`, `Agent`, `Activity`, `Extra info`, and a
duration. The engine hands back rows plus a span per lane; the CLI writes the CSV and the API aggregates.

**Rows tile their lane.** Within a lane, each row starts where the last one ended, the first starts when the lane
started, and the last ends when the lane ended. Nothing is unaccounted for and nothing is counted twice. The only
exception is a batch of parallel tool calls, which genuinely overlap, and every row of one says so in `Overlapped`.

The property holds across every session on the machine this was built against: 720 sessions, 3,879 lanes, 775,849 rows,
no gaps and no negative durations (verified 2026-08-06, `TestRealTimelineSweep`).

## The activity kinds

- **thinking**: a `thinking` block. The span starts when the previous block finished streaming, so it includes API
  queue time and prompt processing, not only reasoning. On a 670k-token context that isn't noise, and it's why the
  legend has to say so rather than calling this "reasoning".
- **writing**: a `text` block, the agent composing prose for its caller.
- **tool call**: composing the call, from the previous block until the `tool_use` block finished streaming.
- **tool execution**: the tool running, from the `tool_use` block until its result came back. This is the honest wall
  clock of the tool, including whatever the tool itself waited on.
- **waiting**: the lane produced nothing, because it was idle on a person, a teammate, or a background task.
- **stalled**: a result that arrived far too late for what the call was doing. The agent was suspended, not working.
- **compacting**: the harness compacting the context.

`compacting` is a seventh kind the original plan didn't have. Compaction took 132 s in the reference session; it isn't
the agent thinking, it isn't a tool running, and folding it into `waiting` would call two minutes of real work
idleness.

## The cursor, and why timestamps can't be trusted

A block's timestamp is when it **finished** streaming, so a span runs from the previous record's timestamp to this
one's. Records go backwards, though: 186 of the reference session's 15,831 do, mostly sub-millisecond write jitter, and
compaction replays a whole run of them two minutes behind.

So the derivation walks a lane with a cursor that only moves forward. A record stamped before the cursor produces a
zero-length row rather than a negative one, and the tiling holds by construction.

## Judgement calls

### A record with several blocks gives its span to the first one

Older transcripts pack a whole response into one record, up to 13 blocks including 11 parallel `tool_use` calls. They
share a single timestamp, so only one of them can honestly claim the span that ended there. It goes to the first block,
which is the thinking block, and that's where the time went. The rest are zero-length.

### Records that aren't work don't cost time

An attachment, a hook's output, a turn summary: they carry timestamps but they aren't the lane doing something, so
their stretch belongs to whichever row surrounds them. Same for a block type nothing decodes, such as `fallback`.

### A turn ending is the only evidence a lane went idle

Without it, a lane that resumes hours later with nothing on record to say what woke it would report hours of
**thinking**, which is badly wrong and completely invisible. So `turn_duration`, `stop_hook_summary`, and
`away_summary` mark the lane as stopped; input arriving (a prompt, or an `enqueue`) says where the next turn's thinking
starts from; and a lane that resumes with neither gets a waiting row and a zero-length block.

Queued input matters more than it looks: 41 of the reference session's 78 idle gaps over five minutes end at an
`enqueue` rather than at a prompt, so watching only prompts misplaces half of them.

### Compaction is placed by its own duration, not by the stamps around it

The boundary record is stamped when compaction finished, while the records replayed after it keep their older stamps.
Measuring back from the boundary with `compactMetadata.durationMs` puts the row where it belongs and leaves the idle
stretch before it as a wait. A boundary reporting no duration marks the moment and claims no time.

### Stall detection, and how hard it tries not to defame honest work

A stall says "the agent was suspended", which is an accusation. The line depends on what the call was doing:

- **An hour** for work with no plausible long form: `rm`, `ls`, `grep`, `git`, a file read, an MCP round trip.
- **Twelve hours** for work that earns its time: a build, a test suite, a checker script, a dev server, a poll loop, or
  spawning a teammate.

Three kinds of call are never stalls whatever they cost. One the harness timed out was ended on purpose. One whose
result never arrived has no end to measure, because it was closed at the moment we stopped reading. And a tool that
blocks until a person answers (`AskUserQuestion`, `ExitPlanMode`) is **waiting**, not stalled: before that rule existed,
three of the five stalls in the whole corpus were questions a human took hours to answer.

What's left, over 775,849 rows: two stalls, both a trivial shell command that came back hours later (6h15m and 2h54m).
The sweep prints every one it finds, so a wrong call is visible rather than buried in a count.

Reading the command matters as much as the threshold. `until pgrep …; do sleep 3; done` is a poll loop and takes the
generous line; `pkill …; sleep 3; pgrep …` is not, because the sleep isn't what the command is for.

### Timeouts are read, then inferred

`timedOutAfterMs` settles it where the harness records it. Where it doesn't, a call that ran for as long as it was
allowed and came back within a minute of that hit its limit; in the reference session that second rule doubles what the
first one finds. A call with no requested limit is left alone: the harness default is two minutes, so inferring from it
would put an ordinary two-minute command on the line.

### Thinking rows borrow their subject, and say so

The thinking text is stripped from 5,469 of 5,471 sampled blocks. Where it survives the row quotes it; everywhere else
the row borrows from what the agent did next, always starting with "before", so a reader can tell a quote from a guess.

### Waiting rows name what ended them

A teammate (read from the harness's envelope, which carries the sender's id), a background task, or a person typing.
Lead waits also list the teammates alive at the time, which is the difference between "blocked on four agents" and
"blocked on nobody". It's a sweep over lanes sorted by start with a heap of the ones still running, because one session
in the corpus has 977 workflow lanes and a scan per row would not do.

## What this doesn't do

- It can't say when a suspended agent woke up, only that the result arrived late.
- It can't split a multi-block record's span across its blocks, because there's only one timestamp.
- A thinking span includes model latency and always will.
- Stall detection is a heuristic. Every stalled row carries the command, the duration, and the line it was measured
  against, so a reader can overrule it.
