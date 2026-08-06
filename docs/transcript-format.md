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
- A session that spawned subagents also has a sibling directory `<session-id>/` holding:
    - `subagents/agent-a<something>.jsonl`: one transcript per subagent lane.
    - `subagents/agent-a<something>.meta.json`: the lane's metadata. Often missing; see below.
    - `subagents/workflows/wf_<id>/agent-a<something>.jsonl`: the same thing for agents a workflow spawned, one level
      deeper, with their own `.meta.json` files. These are real lanes doing real work and it's easy to miss them: one
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

Block types: `thinking`, `text`, `tool_use`.

- `thinking` carries `thinking` and `signature`. **`thinking` is empty in almost every transcript**: 5,469 of 5,471
  sampled blocks (verified 2026-08-06). Only the signature survives. But it is not always empty: two blocks on `2.1.177`
  carry real reasoning text, so read the field and use it when it's there rather than assuming.
- `text` is the agent's prose to its caller.
- `tool_use` carries `id`, `name`, and `input` (tool-specific object). `caller` says `{"type":"direct"}` for a normal
  call.

### `user`

Either a real prompt or a tool result, told apart by the shape of `message.content`:

- A **string**: the user typed it.
- An **array**: blocks of `tool_result` (`tool_use_id`, `content`, sometimes `is_error`), `text`, or `image`. Mostly
  length 1, occasionally up to five (verified 2026-08-06). A record holding a `text` block is still a real prompt.

A tool-result record also carries `toolUseResult` alongside `message` with the structured payload (for `Bash`:
`stdout`, `stderr`, `interrupted`, and friends), and `sourceToolAssistantUUID` pointing back at the `tool_use` record.
Pair on `tool_use_id`; `sourceToolAssistantUUID` is a second, coarser link.

`isMeta: true` marks a harness-injected user record (the `<local-command-caveat>` preamble, for instance) rather than
something a person typed. Three of 388 user records in the reference session. Timeline code should not count these as
prompts.

### `attachment`

Hook output, task reminders, memory injections, file edits, listing deltas. Carries `attachment.type`, seen as
`hook_success` (by far the most), `hook_additional_context`, `async_hook_response`, `task_reminder`, `nested_memory`,
`edited_text_file`, `file`, `date_change`, `skill_listing`, `agent_listing_delta`, `deferred_tools_delta`,
`mcp_instructions_delta`. Not agent work: the timeline shouldn't spend time on these, but the parser must not trip over
them.

### `system`

`subtype` of `turn_duration` (with `durationMs` and `messageCount`), `away_summary` (with `content`, the recap text),
`stop_hook_summary`, `compact_boundary`, `local_command`.

### Titles

Three types carry a session's title, none of them timestamped, all of them rewritten on most turns, so the last of
each in a file wins: `custom-title` (`customTitle`, what a person set), `ai-title` (`aiTitle`, generated), and
`agent-name` (`agentName`, also generated). A session list should prefer them in that order. The reference session
rewrites each of them 109 times, so reading the tail of a file is enough to find them.

### `queue-operation`

`operation` is `enqueue` (with `content`, the user's queued text) or `dequeue`. This is how you spot a prompt that
arrived while the agent was busy, and its `timestamp` is when the user actually typed, not when the agent consumed it.

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
  `relocated` records record the move. The first record's `cwd` is the original project directory.
- Both the lead transcript and its subagent transcripts keep appending while the session is live, so a parse is a
  snapshot. Nothing marks a lane as finished.
