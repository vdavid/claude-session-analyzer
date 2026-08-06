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

The property holds across every session on the machine this was built against: 724 sessions, 3,889 lanes, 780,673 rows,
no gaps and no negative durations (verified 2026-08-06, `TestRealTimelineSweep`).

## The activity kinds

- **thinking**: a `thinking` block. The span starts when the previous block finished streaming, so it includes API
  queue time and prompt processing, not only reasoning. On a 670k-token context that isn't noise, and it's why the
  legend has to say so rather than calling this "reasoning".
- **writing**: a `text` block, the agent composing prose for its caller.
- **tool call**: composing the call, from the previous block until the `tool_use` block finished streaming. When no
  thinking block came first, this span carries the whole response latency, the same caveat `thinking` has.
- **tool execution**: the tool running, from the `tool_use` block until its result came back. This is the honest wall
  clock of the tool, including whatever the tool itself waited on.
- **waiting for a person**: the lane produced nothing until a person typed, queued a prompt, or answered a question the
  agent asked.
- **waiting for a teammate**: another agent's message closed the gap.
- **waiting for a background task**: a background task's notification closed the gap.
- **waiting, reason unknown**: the lane went quiet and later produced something, with no input, message, or
  notification between the two.
- **API error**: the API didn't answer the request. The span is the harness retrying, which is time the session lost
  through no fault of the agent.
- **stalled**: a result that arrived far too late for what the call was doing. The agent was suspended, not working.
- **compacting**: the harness compacting the context.

`Kind.IsWaiting()` is the four waits, and `Kind.IsGap()` adds `stalled` and `API error`: everything where the lane
produced nothing, which is what a swimlane draws as a hole. Ask those rather than listing kinds, so a new one can't be
missed.

Two kinds the original plan didn't have. `compacting`, because compaction took 132 s in the reference session and it's
neither the agent thinking nor a tool running, so folding it into a wait would call two minutes of real work idleness.
And `API error`, for the reason below.

### Why waiting is four kinds and not one

One bucket answers nothing. The reference session's lead sat idle for 71h24m, and "71 hours of waiting" is not a
finding: the split is, because it says the session was blocked on its human far more than on its agents. Naming them
for what was waited on rather than for who waited keeps a subagent's four minutes of waiting in the same buckets as the
lead's 71 hours.

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

### Telling an idle lane from a thinking one

This is the rule that goes wrong invisibly. A lane that sits silent for hours and then produces a block will report
those hours as **thinking** unless something says otherwise, and a reader has no way to spot it.

Three signals, in order of how much they can be trusted:

1. **A turn ended.** `turn_duration`, `stop_hook_summary`, and `away_summary` all say the lane stopped. 46 of the
   reference session's idle gaps over five minutes sit right after one.
2. **Input arrived.** A prompt, or a `queue-operation: enqueue`, timestamps the moment there was something to work on,
   which is where the next turn's thinking starts. Queued input matters more than it looks: 41 of the reference
   session's 78 long idle gaps end at an `enqueue` rather than at a prompt, so watching only prompts misplaces half of
   them. An enqueue splits the lane whenever no tool call is open, without waiting for a turn-end record, because the
   harness doesn't always write one. The cost is that input arriving while the agent is genuinely composing clips a few
   seconds off the front of that row, which is bounded where the alternative isn't: one lane in the corpus sat silent
   for 7h23m after a text block, took a queued "Go on", and answered three seconds later.
3. **The stretch is too long to be a response.** The backstop for lanes carrying neither of the above, which is what a
   session resumed after `/exit` looks like: 25 days of nothing, then a text block. `DefaultMaxResponseSpan` is 15
   minutes, and the constant carries the distribution that puts it there.

A stretch read as idle gets a waiting row, and the block that closed it claims none of it: nothing in the transcript
says when the lane actually woke up.

### A failed request is its own kind, and is bounded rather than trusted

The harness writes a failed request as an ordinary assistant record with a text block, so nothing but its typed fields
(`docs/transcript-format.md`) tells it from the agent writing that sentence. Read as prose it becomes a `writing` row;
read as a long silence it trips the 15-minute backstop above and becomes idle time. Either way an outage quietly lands
in a number that means something else, which is why it gets a kind.

`API error` covers everything the API refused, not only an outage: a rate limit, an expired login, a prompt too long,
a model that doesn't exist. Naming the kind "outage" would be wrong for half of them, and the row's info carries the
typed reason and the status, so the specific failure is right there.

The span is the stretch before the record, because the retries aren't written down: no transcript in the corpus holds
two error records in a row, so the gap before one is the only measure of the outage there is. Two clamps keep that
honest, and both leave a wait behind rather than swallowing time they can't account for:

- A lane whose turn had already ended had no request in flight, so the stretch before the error stays a wait and the
  error marks the moment without claiming any of it.
- The retry window can't run longer than `DefaultMaxAPIErrorSpan`, which is two hours against a corpus whose longest
  such gap is 1h19m. The case it guards has no evidence and every reason to expect: a session resumed weeks later,
  whose first request fails on a login that expired while it sat there.

A lane is idle after a failed request, because the agent can't carry on by itself. Whatever comes next was started by
something else, and an unexplained gap after one says so.

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

### A wait is attributed by the record that ended it

The record that arrived is the signal: a teammate's message (read from the harness's envelope, which carries the
sender's id), a background task's notification, or a person's prompt. So a notification landing while four teammates
are alive is a background task's wait, not theirs, and a wait nothing arrived to end is `waiting, reason unknown`
rather than a guess.

That leaves no ambiguity to resolve: across the corpus's 12,437 envelope-carrying records, not one carries both
envelopes (verified 2026-08-06). A notification arrives as a plain prompt about a third of the time (2,044 against
6,288 queued), so both paths are read the same way.

The envelope is only looked for in the first 200 characters, which is the message's own wrapper rather than something
it quotes. Three enqueued messages in the corpus carry a notification past that point and are read as a person's
prompt, which is the deliberate side of that trade.

Lead waits also list the teammates alive at the time, which is the difference between "blocked on four agents" and
"blocked on nobody". It's a sweep over lanes sorted by start with a heap of the ones still running, because one session
in the corpus has 977 workflow lanes and a scan per row would not do.

## What this doesn't do

- It can't say when a suspended agent woke up, only that the result arrived late. The same goes for a lane that resumes
  with no input on record: the row before it is idle, and the block that ended it claims no time.
- It can't split a multi-block record's span across its blocks, because there's only one timestamp.
- A thinking span includes model latency and always will, and so does a tool call span with no thinking block before
  it.
- A tool execution span includes anything the tool waited on, including a person answering a permission prompt. Nothing
  in the transcript separates the two.
- Two lanes can carry the same name, because a lane with no `.meta.json` falls back to its agent type. Group by
  `LaneID`, not by `Agent`.
- Stall detection is a heuristic. Every stalled row carries the command, the duration, and the line it was measured
  against, so a reader can overrule it.
