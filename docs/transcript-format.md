# The Claude Code transcript format

What `internal/transcript` and `internal/session` decode. Reverse-engineered from transcripts on disk, not a published
spec, so it drifts: read every claim below as "true for versions named", keep parser tolerant.

Verification base: 57 MB multi-agent session (`532ac591-b7c5-45ca-a764-f40f01a0a9ac`, 26 lanes, 2026-08-03 to
2026-08-06, Claude Code `2.1.220`), plus samples across 200 random transcripts from a 4,438-transcript corpus spanning
`2.1.112` (2026-04) to `2.1.220` (2026-08). Dates below = when a claim was last checked.

Corpus grows while measured, so counts from consecutive sweeps differ by a few. Each number carries its measurement
base; read as scale, not identities to reconcile.

## Layout on disk

- Sessions at `~/.claude/projects/<project-slug>/<session-id>.jsonl`. Override root with `CLAUDE_CONFIG_DIR` (parser
  reads `$CLAUDE_CONFIG_DIR/projects` when set).
- Slug = project path with `/` and `.` replaced by `-`. Lossy: `/tmp/a.b` and `/tmp/a/b` collide. Don't invert it. Read
  `cwd` off any record instead.
- Session with subagents also has directory `<session-id>/`, next to lead transcript or under another slug (see below),
  holding:
    - `subagents/agent-a<something>.jsonl`: one transcript per subagent lane.
    - `subagents/agent-a<something>.meta.json`: lane metadata. Often missing; see below.
    - `subagents/workflows/wf_<id>/agent-a<something>.jsonl`: same for workflow-spawned agents, one level deeper, own
      `.meta.json` files. Real lanes doing real work; missing them costs a lot: one session holds 977 against five
      direct subagents. Agent transcripts live at exactly these two depths, nowhere else (verified 2026-08-06 across all
      3,708).
    - `subagents/workflows/wf_<id>/journal.jsonl`: workflow's own log, not a lane. Records are `started` and `result`
      (`agentId`, `key`, and for `result` agent's final report text), none timestamped. Taking only `agent-*.jsonl`
      leaves it out.
    - `workflows/wf_<id>.json` and `workflows/scripts/*.js`: workflow definitions and scripts they run. Not transcripts.
    - `tool-results/*.txt`: offloaded large tool outputs. No record in verified session references these, so treat
      directory as opaque, ignore for timing (verified 2026-08-06 by scanning every record of reference session for
      string `tool-results/`).
- Sibling `.jsonl.wakatime` files come from an unrelated tool. Ignore.
- Scale to design for: 4,460 transcripts, 3.8 GB, one developer's machine after four months (2026-08-06). Session
  listing must not parse bodies.

### A session's lanes are not always next to its lead transcript

Slug a lane is written under = directory session sits in **at the time**, not where it started. Session entering a git
worktree keeps lead transcript where it started while `<session-id>/subagents/` appears under worktree's slug; session
that moved several times has a directory under several slugs, up to 12 (verified 2026-08-06). 452 lane files here sit
under a slug with no lead transcript, in 31 sessions, disproportionately the big multi-agent efforts.

Directory ties to session by **name**, not by surrounding slug:

- Session id names exactly one session across whole root: 724 lead transcripts, no id under two slugs.
- Every uuid-named directory has a lead transcript somewhere. Only non-session-named directory under a slug is `memory`.
- 400 of 400 sampled lane files under a non-lead slug carry directory's name as `sessionId`, the parent session's id, so
  records confirm the tie rather than slug shape suggesting it.

(All four verified 2026-08-06 by scanning every directory under root.)

### A lane can be split across several of them

Harness appends to whichever directory session sits in, switching back and forth, so one lane arrives as two or three
fragments sharing a name under different slugs. Seven lanes split in corpus, one three ways (verified 2026-08-06).

Fragments share no records and **interleave in time**: one lane's middle fragment spans 19:54:32 to 20:03:52 while a
third runs 19:54:39 to 19:56:22 inside it. Merging by timestamp reproduces the `parentUuid` chain exactly on all seven
(one head, no record before its parent); concatenating in slug order breaks it on four. So lane = its path inside a
session directory (`subagents/agent-<id>.jsonl`), not the file it sits in: same path under two slugs is one lane in two
pieces, and reading it as two hands back two lanes carrying one agent id.

Exactly one fragment carries the `.meta.json`, unpredictably which, so read lane metadata off whichever file has one
(verified 2026-08-06 across all seven split lanes).

### Subagent file names are not parseable

Name = `agent-a` + opaque suffix. Newer transcripts: suffix looks like `<name>-<hash>`, but name may contain dashes, so
splitting is ambiguous. `agent-aplan-reviewer-2c89a8fbeba6e094` is `plan-reviewer` with hash `2c89a8fbeba6e094`;
`agent-aplan-reviewer-2-effb0cdee8711d69` is `plan-reviewer-2`. Older transcripts (2026-04) carry no name:
`agent-aa5934dc7850f0f60.jsonl`.

So: take lane label from `.meta.json`, fall back to file base name only as last resort.

### `.meta.json` fields vary by version

A 2026-08 file has everything:

```json
{
    "agentType": "m1-engine",
    "description": "Build the repo skeleton and the reader",
    "name": "m1-engine",
    "spawnDepth": 0,
    "model": "claude-opus-5",
    "taskKind": "in_process_teammate",
    "teamName": "session-532ac591",
    "color": "blue",
    "planModeRequired": false,
    "permissionMode": "bypassPermissions"
}
```

A 2026-04 file has two:

```json
{ "agentType": "general-purpose", "description": "Build-time DB bundling" }
```

File is sometimes absent entirely, including recent sessions (verified 2026-08-06: several 2026-06 and 2026-07 sessions
have `subagents/*.jsonl` with no sibling `.meta.json`). Every field optional. Lane labelling falls back `name` →
`agentType` → file's agent id, and `color` may be empty, so UI needs its own palette fallback.

## Records

One JSON object per line. Lines get long: 1.36 MB longest in reference session, 3.42 MB longest in corpus (verified
2026-08-06), so `bufio.Scanner` at its 64 KB default fails on real data. `internal/transcript` reads lines with a
growing `bufio.Reader` loop, no fixed ceiling.

Not every line is a message. Types seen across corpus (verified 2026-08-06, 150 random transcripts):

- Messages: `assistant`, `user`, `attachment`, `system`.
- Session state, no timestamp on most: `custom-title`, `ai-title`, `agent-name`, `mode`, `permission-mode`,
  `agent-setting`, `last-prompt`, `bridge-session`, `worktree-state`, `relocated`, `pr-link`, `fork-context-ref`.
- File tracking: `file-history-snapshot`, `file-history-delta`.
- User input queue: `queue-operation`.
- Workflow journals only: `started`, `result`.

Complete set as of 2026-08-06, from parsing all 4,460 transcripts (1,236,738 lines, 0 malformed). New types appear over
time (`ai-title`, `relocated`, `pr-link` all newer than `2.1.112`), so an unknown `type` must be skipped, never treated
as an error. `internal/session`'s `TestRealCorpusSweep` re-runs that check on demand and lists what it skipped, which is
how a new type gets noticed.

### Fields shared by message records

`uuid`, `parentUuid` (previous record in lane, `null` at head), `timestamp` (RFC 3339, UTC), `sessionId`, `cwd`,
`gitBranch`, `version`, `isSidechain`, `userType`, `entrypoint`. Subagent records add `agentId`. `sessionId` on a
subagent record is the **parent** session's id, so it can't distinguish lanes: use file path.

Non-message records often carry no `timestamp` (`custom-title`, `mode`, `permission-mode`, `bridge-session`, ...), so
zero timestamp is normal and must not be read as 1970.

### `assistant`

Carries `message` (API response), `requestId` grouping blocks of one response, and `message.usage` with token counts.
Earlier blocks of a request hold partial usage; final block holds total.

**`message.content` is an array, and it is usually but not always length 1.** In reference session all 570 assistant
records hold exactly one block, but across 200 random transcripts 10 records of 32,655 hold more (up to 13: one
`thinking`, one `text`, 11 parallel `tool_use`, on `2.1.150` and `2.1.181`). Parse per block, not per record.

Block types: `thinking`, `text`, `tool_use`, rarely `fallback`.

- `thinking` carries `thinking` and `signature`. **`thinking` is empty in almost every transcript**: 5,469 of 5,471
  sampled blocks (verified 2026-08-06). Only signature survives. But not always empty: two blocks on `2.1.177` carry
  real reasoning text, so read the field and use it when present rather than assuming.
- `text`: agent's prose to its caller.
- `tool_use` carries `id`, `name`, `input` (tool-specific object). `caller` says `{"type":"direct"}` for a normal call.
  A `Bash` call's `input.timeout` is the limit it asked for, in milliseconds.
- `fallback` marks harness switching models mid-response:
  `{"type":"fallback","from":{"model":...},"to":{"model": ...}}`. No text, not the agent doing anything. Two blocks in
  250 random transcripts (verified 2026-08-06). New block types will appear, so an unknown one must be stepped over, not
  treated as content.

### An assistant record can be the API failing rather than the agent answering

When a request doesn't come back, harness writes what looks like an ordinary assistant record: one `text` block of prose
for the person at the terminal, `model: "<synthetic>"`, zeros throughout `usage`. Three fields tell it apart, and only
they should be read (prose is copy and changes):

- `isApiErrorMessage: true`, always present on one.
- `error`, typed string. Across corpus: `rate_limit` (70), `authentication_failed` (57), `server_error` (46),
  `invalid_request` (43), `unknown` (27), `model_not_found` (2). Always present.
- `apiErrorStatus`, HTTP status as bare number: 429, 401, 529, 500, 413, 404. **Missing on 76 of the 245**, so a record
  with no status is ordinary.

Six of the 245 also carry `errorDetails`, which nothing reads yet.

Scale and spread (verified 2026-08-06, the 4,445 transcripts here): 245 records in 185 of them, across 38 harness
versions from `2.1.138` to `2.1.221`, in lead and subagent lanes alike. Never in runs: no transcript holds two in a row,
so retries a long outage costs are invisible and only the gap before the record measures them. That gap is under a
minute for 191, longest 1h19m.

`isApiErrorMessage: false` appears on ordinary assistant records too, so the flag's presence means nothing alone.

### `user`

Either a real prompt or a tool result, told apart by shape of `message.content`:

- **String**: user typed it.
- **Array**: blocks of `tool_result` (`tool_use_id`, `content`, sometimes `is_error`), `text`, or `image`. Mostly length
  1, occasionally up to five (verified 2026-08-06). A record holding a `text` block is still a real prompt.

Tool-result record also carries `toolUseResult` alongside `message` with the structured payload, and
`sourceToolAssistantUUID` pointing back at the `tool_use` record. Pair on `tool_use_id`; `sourceToolAssistantUUID` is a
second, coarser link.

**`toolUseResult` is not always an object.** Across 200 random transcripts: object 24,194 times, bare string 1,473,
array 784 (verified 2026-08-06), so reading a field out of it must tolerate all three. A `Bash` payload object carries
`stdout`, `stderr`, `interrupted`, `isImage`, `noOutputExpected`.

### Reading a timeout off a tool result

A `Bash` call the harness cut short at its timeout arrives in one of two shapes, only the first machine-readable:

- Payload object carries `timedOutAfterMs` (limit enforced) and `backgroundTaskId` (command moved to background rather
  than stopped). Nine of reference session's 18 calls that ran to their limit carry it.
- Payload is a bare string, `"Error: Exit code 143\nCommand timed out after 10m 0s"`, with `is_error: true` on the
  result block. Nothing structured says what the limit was.

Harness caps a `Bash` call at ten minutes whatever it asks for: four calls in reference session requesting 1,200 s to
3,600 s all came back at 600.1 s (verified 2026-08-06). That's the `BASH_MAX_TIMEOUT_MS` default and a deployment can
raise it, so treat as a ceiling on inference, not a fact.

### The `Agent` tool's result links a spawn to the lane it created

Teammate spawn comes back with `status: "teammate_spawned"`, `agent_id` (`<name>@<team>`), `name`, `agent_type`,
`model`, `color`. Direct link from call to lane it started, which beats matching on file name: `agent_id`'s local part
is the lane's `.meta.json` `name`, and file names can't be split reliably.

### Input from outside a lane arrives in an envelope

Harness wraps relayed input in a tag of its own: structure rather than prose, so it parses.

- `<teammate-message teammate_id="m1-engine" color="green">…</teammate-message>`: message from another agent. Subagent
  gets it as whole prompt; lead gets one line of preamble first, `Another Claude session sent a message:`.
- `<task-notification><task-id>…</task-id><tool-use-id>…</tool-use-id><output-file>…</output-file><status>…</status><summary>…</summary></task-notification>`:
  background task reporting in. Arrives as `queue-operation` content, and about a third of the time as a prompt (2,044
  against 6,288 queued, verified 2026-08-06). `tool-use-id` is the `tool_use` id of the call that started the task, so
  it links a notification to the lane that owns it, whichever lane the notification landed in. 9,884 of the corpus's
  12,076 notifications carry one; 1,726 of the rest are `Monitor` events and 279 carry `output-file` straight after
  `task-id` (verified 2026-08-07).

`isMeta: true` marks a harness-injected user record rather than something a person typed: `<local-command-caveat>`
preamble, or a `<system-reminder>` block. Three of 388 user records in reference session. Timeline code should not count
these as prompts.

### `attachment`

Hook output, task reminders, memory injections, file edits, listing deltas. Carries `attachment.type`, seen as
`hook_success` (by far the most), `hook_additional_context`, `async_hook_response`, `task_reminder`, `nested_memory`,
`edited_text_file`, `file`, `date_change`, `skill_listing`, `agent_listing_delta`, `deferred_tools_delta`,
`mcp_instructions_delta`. Not agent work: timeline shouldn't spend time on these, but parser must not trip over them.

### `system`

`subtype` of `turn_duration` (with `durationMs` and `messageCount`), `away_summary` (with `content`, the recap text),
`stop_hook_summary`, `compact_boundary`, `local_command`.

`turn_duration`, `stop_hook_summary`, and `away_summary` all mark a turn ending, the only record saying the lane went
idle rather than kept thinking. Of reference session's gaps over five minutes that aren't a tool running, 46 sit right
after one of these.

**Subagent lanes carry almost no `system` records at all**: two `compact_boundary` records across 300 sampled subagent
transcripts, nothing else (verified 2026-08-06). No `queue-operation` records either. So a subagent's idle time can only
be bounded by prompts it receives.

### Titles

Three types carry a session's title, none timestamped, all rewritten on most turns, so last of each in a file wins:
`custom-title` (`customTitle`, person-set), `ai-title` (`aiTitle`, generated), `agent-name` (`agentName`, also
generated). Session list should prefer that order. Reference session rewrites each 109 times, so reading a file's tail
is enough to find them.

### `queue-operation`

`operation` is `enqueue`, `remove`, `dequeue`, or `popAll`, in that order of frequency (3,378 / 2,098 / 1,144 / 55
across 200 random transcripts, verified 2026-08-06). Only `enqueue` carries `content`, the text that arrived.

This is how you spot input landing while the agent was busy. An `enqueue` timestamp is when input actually arrived, not
when the agent consumed it. In reference session, 41 of 78 idle gaps over five minutes end at an `enqueue` rather than a
prompt, so a timeline watching only prompts misplaces half of them.

## Timing semantics

- Block's `timestamp` = when it **finished streaming**, not when it started. So a span runs from previous record's
  timestamp to this record's.
- So a `thinking` span unavoidably includes API queue time and prompt processing. On a large context that isn't noise.
  Label it honestly; don't present it as pure reasoning time.
- Tool execution is exact: `tool_use` timestamp to matching `tool_result` timestamp, paired on `tool_use_id`.

### Timestamps go backwards, so never assume monotonic

186 of 15,831 records in reference session are stamped earlier than the record before them in the same lane (verified
2026-08-06). Two distinct causes:

- **Write jitter**, the common case: median 1 ms, maximum 121 ms, between records written by different code paths in the
  same instant. Harmless, but makes durations negative.
- **Compaction**, the one that matters: a single 132.015 s backward jump. Compaction summary record is stamped when
  compaction finished, while records replayed after it are stamped when it started. Neighbouring `system` /
  `compact_boundary` record confirms it, carrying `compactMetadata.durationMs: 132016` plus `preTokens`, `postTokens`,
  `trigger`.

So: clamp negative spans to zero, and read compaction time off `compactMetadata.durationMs` rather than deriving it from
surrounding stamps.

### Session-level facts

- A session's records claim several `cwd` values when it entered a git worktree: `worktree-state` and `relocated`
  records record the move. First record's `cwd` is the original project directory, and the move is also what scatters
  the session's lanes across slugs (see layout section above).
- Lead transcript and its subagent transcripts keep appending while the session is live, so a parse is a snapshot.
  Nothing marks a lane as finished.
