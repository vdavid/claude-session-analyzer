# The Claude Code transcript format

What `internal/transcript` and `internal/session` decode. This is reverse-engineered from transcripts on disk, not from
a published spec, so it drifts: treat every claim below as "true for the versions named", and keep the parser tolerant.

Verification base: a 57 MB multi-agent session (`532ac591-b7c5-45ca-a764-f40f01a0a9ac`, 26 lanes, 2026-08-03 to
2026-08-06, Claude Code `2.1.220`), plus samples across 200 random transcripts from a 4,438-transcript corpus spanning
`2.1.112` (2026-04) to `2.1.220` (2026-08). Dates below are when a claim was last checked.

## Layout on disk

- Sessions live at `~/.claude/projects/<project-slug>/<session-id>.jsonl`. Override the root with `CLAUDE_CONFIG_DIR`
  (the parser reads `$CLAUDE_CONFIG_DIR/projects` when that's set).
- The slug is the project path with `/` and `.` replaced by `-`, which is lossy: `/tmp/a.b` and `/tmp/a/b` collide.
  Don't invert it. Read `cwd` off any record instead.
- A session that spawned subagents also has a directory `<session-id>/`, next to the lead transcript or under
  another slug (see below), holding:
    - `subagents/agent-a<something>.jsonl`: one transcript per subagent lane.
    - `subagents/agent-a<something>.meta.json`: the lane's metadata. Often missing; see below.
    - `subagents/workflows/wf_<id>/agent-a<something>.jsonl`: the same thing for agents a workflow spawned, one level
      deeper, with their own `.meta.json` files. These are real lanes doing real work, and missing them costs a lot: one
      session in the corpus holds 977 of them against five direct subagents. Agent transcripts live at exactly these
      two depths, nowhere else (verified 2026-08-06 across all 3,708 of them).
    - `subagents/workflows/wf_<id>/journal.jsonl`: the workflow's own log, not a lane. Its records are `started` and
      `result` (`agentId`, `key`, and for `result` the agent's final report text), none of them timestamped. Taking
      only `agent-*.jsonl` leaves it out.
    - `workflows/wf_<id>.json` and `workflows/scripts/*.js`: workflow definitions and the scripts they run. Not
      transcripts.
    - `tool-results/*.txt`: offloaded large tool outputs. No record in the verified session references these files, so
      treat the directory as opaque and ignore it for timing (verified 2026-08-06 by scanning every record of the
      reference session for the string `tool-results/`).
- Sibling `.jsonl.wakatime` files come from an unrelated tool. Ignore them.
- Scale to design for: 4,438 transcripts, 3.8 GB, on one developer's machine after four months. Session listing must
  not parse bodies.

### A session's lanes are not always next to its lead transcript

The slug a lane is written under is the directory the session is in **at the time**, not the one it started in. So a
session that enters a git worktree keeps its lead transcript where it started while `<session-id>/subagents/` appears
under the worktree's slug, and a session that moved several times has a directory under several slugs, up to 12
(verified 2026-08-06). 452 lane files on this machine sit under a slug holding no lead transcript, in 31 sessions, and
they're disproportionately the big multi-agent efforts.

What ties a directory to a session is its **name**, not the slug around it:

- A session id names exactly one session across the whole root: 724 lead transcripts, no id under two slugs.
- Every uuid-named directory in the corpus has a lead transcript somewhere. The only directory under a slug that isn't
  named after a session is `memory`.
- 400 of 400 sampled lane files under a non-lead slug carry the directory's name as their `sessionId`, which is the
  parent session's id, so the records confirm the tie rather than the slug's shape suggesting it.

(All four verified 2026-08-06 by scanning every directory under the root.)

### A lane can be split across several of them

The harness appends to whichever directory the session is in and switches back and forth, so one lane arrives as two or
three fragments carrying the same name under different slugs. Seven lanes in the corpus are split, one of them three
ways (verified 2026-08-06).

The fragments share no records, and they **interleave in time**: one lane's middle fragment spans 19:54:32 to 20:03:52
while a third runs 19:54:39 to 19:56:22 inside it. Merging them by timestamp reproduces the `parentUuid` chain exactly
on all seven (one head, no record before its parent), while concatenating them in slug order breaks it on four. So a
lane is its path inside a session directory (`subagents/agent-<id>.jsonl`), not the file it sits in: the same path
under two slugs is one lane in two pieces, and reading it as two lanes hands back two lanes carrying one agent id.

Exactly one fragment carries the `.meta.json`, and which one isn't predictable, so read the lane's metadata off
whichever of its files has one (verified 2026-08-06 across all seven split lanes).

### Subagent file names are not parseable

The name is `agent-a` + an opaque suffix. In newer transcripts the suffix looks like `<name>-<hash>`, but the name may
itself contain dashes, so splitting is ambiguous: `agent-aplan-reviewer-2c89a8fbeba6e094` is `plan-reviewer` with hash
`2c89a8fbeba6e094`, while `agent-aplan-reviewer-2-effb0cdee8711d69` is `plan-reviewer-2`. Older transcripts (2026-04)
carry no name at all: `agent-aa5934dc7850f0f60.jsonl`.

So: take the lane label from `.meta.json`, and fall back to the file's base name only as a last resort.

### `.meta.json` fields vary by version

A 2026-08 file has everything:

```json
{"agentType":"m1-engine","description":"Build M1 repo skeleton and parser","name":"m1-engine","spawnDepth":0,
 "model":"claude-opus-5","taskKind":"in_process_teammate","teamName":"session-532ac591","color":"blue",
 "planModeRequired":false,"permissionMode":"bypassPermissions"}
```

A 2026-04 file has two:

```json
{"agentType":"general-purpose","description":"Build-time DB bundling"}
```

And the file is sometimes absent entirely, including in recent sessions (verified 2026-08-06: several 2026-06 and
2026-07 sessions have `subagents/*.jsonl` with no sibling `.meta.json`). Every field is optional. Lane labelling falls
back `name` → `agentType` → the file's agent id, and `color` may be empty, so the UI needs its own palette fallback.

## Records

One JSON object per line. Lines get long: 1.36 MB is the longest in the reference session, and 3.42 MB is the longest
in the whole corpus (verified 2026-08-06), so a
`bufio.Scanner` at its default 64 KB limit fails on real data. `internal/transcript` reads lines with a growing
`bufio.Reader` loop instead, with no fixed ceiling.

Not every line is a message. Types seen across the corpus (verified 2026-08-06, 150 random transcripts):

- Messages: `assistant`, `user`, `attachment`, `system`.
- Session state, no timestamp on most: `custom-title`, `ai-title`, `agent-name`, `mode`, `permission-mode`,
  `agent-setting`, `last-prompt`, `bridge-session`, `worktree-state`, `relocated`, `pr-link`, `fork-context-ref`.
- File tracking: `file-history-snapshot`, `file-history-delta`.
- User input queue: `queue-operation`.
- Workflow journals only: `started`, `result`.

That list is the complete set as of 2026-08-06, from parsing all 4,438 transcripts (1,221,828 lines, 0 malformed).
New types appear over time (`ai-title`, `relocated`, and `pr-link` are all newer than `2.1.112`), so an unknown `type`
must be skipped, never treated as an error. `internal/session`'s `TestRealCorpusSweep` re-runs that check on demand
and lists what it skipped, which is how a new type gets noticed.

### Fields shared by message records

`uuid`, `parentUuid` (the previous record in the lane, `null` at the head), `timestamp` (RFC 3339, UTC),
`sessionId`, `cwd`, `gitBranch`, `version`, `isSidechain`, `userType`, `entrypoint`. Subagent records add `agentId`.
`sessionId` on a subagent record is the **parent** session's id, so it can't distinguish lanes: use the file path.

Records that aren't messages often carry no `timestamp` at all (`custom-title`, `mode`, `permission-mode`,
`bridge-session`, ...), so a zero timestamp is normal and must not be read as 1970.

### `assistant`

Carries `message` (the API response), `requestId` grouping the blocks of one response, and `message.usage` with token
counts. Earlier blocks of a request hold partial usage; the final block holds the total.

**`message.content` is an array, and it is usually but not always length 1.** In the reference session all 570
assistant records hold exactly one block, but across 200 random transcripts 10 records of 32,655 hold more (up to 13
blocks: one `thinking`, one `text`, and 11 parallel `tool_use` blocks, on `2.1.150` and `2.1.181`). Parse per block, not
per record.

Block types: `thinking`, `text`, `tool_use`, and rarely `fallback`.

- `thinking` carries `thinking` and `signature`. **`thinking` is empty in almost every transcript**: 5,469 of 5,471
  sampled blocks (verified 2026-08-06). Only the signature survives. But it is not always empty: two blocks on `2.1.177`
  carry real reasoning text, so read the field and use it when it's there rather than assuming.
- `text` is the agent's prose to its caller.
- `tool_use` carries `id`, `name`, and `input` (tool-specific object). `caller` says `{"type":"direct"}` for a normal
  call. A `Bash` call's `input.timeout` is the limit it asked for, in milliseconds.
- `fallback` marks the harness switching models mid-response: `{"type":"fallback","from":{"model":...},"to":{"model":
  ...}}`. It carries no text and isn't the agent doing anything. Two blocks in 250 random transcripts (verified
  2026-08-06). New block types will appear, so an unknown one has to be stepped over rather than treated as content.

### An assistant record can be the API failing rather than the agent answering

When a request doesn't come back, the harness writes what looks like an ordinary assistant record: one `text` block
holding prose for the person at the terminal, `model: "<synthetic>"`, and zeros throughout `usage`. Three fields tell
it apart, and only they should be read (the prose is copy and changes):

- `isApiErrorMessage: true`, always present on one.
- `error`, a typed string. Across the corpus: `rate_limit` (70), `authentication_failed` (57), `server_error` (46),
  `invalid_request` (43), `unknown` (27), `model_not_found` (2). Always present.
- `apiErrorStatus`, an HTTP status as a bare number: 429, 401, 529, 500, 413, 404. **Missing on 76 of the 245**, so a
  record with no status is ordinary.

Six of the 245 also carry `errorDetails`, which nothing reads yet.

Scale and spread (verified 2026-08-06, the 4,445 transcripts on this machine): 245 records in 185 of them, across 38
harness versions
from `2.1.138` to `2.1.221`, in lead and subagent lanes alike. They never come in runs: no transcript holds two in a
row, so the retries a long outage costs are invisible and only the gap before the record measures them. That gap is
under a minute for 191 of them, and its longest is 1h19m.

`isApiErrorMessage: false` appears on ordinary assistant records too, so the flag's presence means nothing on its own.

### `user`

Either a real prompt or a tool result, told apart by the shape of `message.content`:

- A **string**: the user typed it.
- An **array**: blocks of `tool_result` (`tool_use_id`, `content`, sometimes `is_error`), `text`, or `image`. Mostly
  length 1, occasionally up to five (verified 2026-08-06). A record holding a `text` block is still a real prompt.

A tool-result record also carries `toolUseResult` alongside `message` with the structured payload, and
`sourceToolAssistantUUID` pointing back at the `tool_use` record. Pair on `tool_use_id`; `sourceToolAssistantUUID` is a
second, coarser link.

**`toolUseResult` is not always an object.** Across 200 random transcripts it is an object 24,194 times, a bare string
1,473 times, and an array 784 times (verified 2026-08-06), so reading a field out of it has to tolerate all three. A
`Bash` payload object carries `stdout`, `stderr`, `interrupted`, `isImage`, and `noOutputExpected`.

### Reading a timeout off a tool result

A `Bash` call the harness cut short at its timeout arrives in one of two shapes, and only the first is machine-readable:

- The payload object carries `timedOutAfterMs` (the limit that was enforced) and `backgroundTaskId` (the command was
  moved to the background rather than stopped). Nine of the reference session's 18 calls that ran to their limit carry
  it.
- The payload is a bare string, `"Error: Exit code 143\nCommand timed out after 10m 0s"`, with `is_error: true` on the
  result block. Nothing structured says what the limit was.

The harness caps a `Bash` call at ten minutes whatever it asks for: four calls in the reference session requesting
1,200 s to 3,600 s all came back at 600.1 s (verified 2026-08-06). That's the `BASH_MAX_TIMEOUT_MS` default and a
deployment can raise it, so treat it as a ceiling on inference rather than a fact.

### The `Agent` tool's result links a spawn to the lane it created

A teammate spawn comes back with `status: "teammate_spawned"`, `agent_id` (`<name>@<team>`), `name`, `agent_type`,
`model`, and `color`. That's a direct link from the call to the lane it started, which beats matching on the file name:
`agent_id`'s local part is the lane's `.meta.json` `name`, and the file name can't be split reliably.

### Input from outside a lane arrives in an envelope

The harness wraps input it relays in a tag of its own, which is structure rather than prose and can be parsed:

- `<teammate-message teammate_id="m1-engine" color="green">…</teammate-message>` is a message from another agent. A
  subagent gets it as the whole prompt; a lead gets one line of preamble first, `Another Claude session sent a
  message:`.
- `<task-notification><task-id>…</task-id>…</task-notification>` is a background task reporting in. It arrives as
  `queue-operation` content rather than as a prompt.

`isMeta: true` marks a harness-injected user record rather than something a person typed: the
`<local-command-caveat>` preamble, or a `<system-reminder>` block. Three of 388 user records in the reference session.
Timeline code should not count these as prompts.

### `attachment`

Hook output, task reminders, memory injections, file edits, listing deltas. Carries `attachment.type`, seen as
`hook_success` (by far the most), `hook_additional_context`, `async_hook_response`, `task_reminder`, `nested_memory`,
`edited_text_file`, `file`, `date_change`, `skill_listing`, `agent_listing_delta`, `deferred_tools_delta`,
`mcp_instructions_delta`. Not agent work: the timeline shouldn't spend time on these, but the parser must not trip over
them.

### `system`

`subtype` of `turn_duration` (with `durationMs` and `messageCount`), `away_summary` (with `content`, the recap text),
`stop_hook_summary`, `compact_boundary`, `local_command`.

`turn_duration`, `stop_hook_summary`, and `away_summary` all mark a turn ending, which is the only record saying the
lane went idle rather than kept thinking. Of the reference session's gaps over five minutes that aren't a tool running,
46 sit right after one of these.

**Subagent lanes carry almost no `system` records at all**: two `compact_boundary` records across 300 sampled subagent
transcripts, and nothing else (verified 2026-08-06). They carry no `queue-operation` records either. So a subagent's
idle time can only be bounded by the prompts it receives.

### Titles

Three types carry a session's title, none of them timestamped, all of them rewritten on most turns, so the last of
each in a file wins: `custom-title` (`customTitle`, what a person set), `ai-title` (`aiTitle`, generated), and
`agent-name` (`agentName`, also generated). A session list should prefer them in that order. The reference session
rewrites each of them 109 times, so reading the tail of a file is enough to find them.

### `queue-operation`

`operation` is `enqueue`, `remove`, `dequeue`, or `popAll`, in that order of frequency (3,378 / 2,098 / 1,144 / 55
across 200 random transcripts, verified 2026-08-06). Only `enqueue` carries `content`, the text that arrived.

This is how you spot input that landed while the agent was busy, and an `enqueue` timestamp is when the input actually
arrived, not when the agent consumed it. In the reference session, 41 of the 78 idle gaps over five minutes end at an
`enqueue` rather than at a prompt, so a timeline that only watches prompts misplaces half of them.

## Timing semantics

- A block's `timestamp` is when the block **finished streaming**, not when it started. A span therefore runs from the
  previous record's timestamp to this record's timestamp.
- So a `thinking` span unavoidably includes API queue time and prompt processing. On a large context that isn't noise.
  Label it honestly; don't present it as pure reasoning time.
- Tool execution is exact: `tool_use` timestamp to the matching `tool_result` timestamp, paired on `tool_use_id`.

### Timestamps go backwards, so never assume monotonic

186 of 15,831 records in the reference session are stamped earlier than the record before them in the same lane
(verified 2026-08-06). Two distinct causes:

- **Write jitter**, the common case: a median of 1 ms and a maximum of 121 ms, between records written by different
  code paths in the same instant. Harmless, but it makes durations negative.
- **Compaction**, the one that matters: a single 132.015 s backward jump. The compaction summary record is stamped when
  compaction finished, while the records replayed after it are stamped when it started. The neighbouring
  `system` / `compact_boundary` record confirms it, carrying `compactMetadata.durationMs: 132016` along with
  `preTokens`, `postTokens`, and `trigger`.

So: clamp negative spans to zero, and read compaction time off `compactMetadata.durationMs` rather than trying to
derive it from the surrounding stamps.

### Session-level facts

- A session's own records claim several `cwd` values when the session entered a git worktree: `worktree-state` and
  `relocated` records record the move. The first record's `cwd` is the original project directory, and the move is also
  what scatters the session's lanes across slugs (see the layout section above).
- Both the lead transcript and its subagent transcripts keep appending while the session is live, so a parse is a
  snapshot. Nothing marks a lane as finished.
