# The HTTP API

What `claude-session-analyzer serve` answers, and what the web app is built against. `internal/api` renders it; changing
a field there means changing this.

The server binds to `127.0.0.1` and nothing else. A tool that reads every transcript on the machine has no business on
the network, so the address isn't configurable.

## Conventions

- Every instant is RFC 3339 in UTC. A timestamp that isn't known is `null`, never a zero date: 99 of the 725 sessions on
  the machine this was built against carry no timestamped record at all (verified 2026-08-06).
- Every duration is seconds, as a number, named `seconds` or `...Seconds`, rounded to the millisecond the transcripts
  are stamped at.
- A session id can be a prefix, as long as it matches one session. That's the same rule the CLI uses.
- Nothing is cached. A session is parsed per request, which is under a second for all but the largest.
- Errors are `{"error": {"code", "message", "matches"}}`. `code` is what a caller branches on, `message` is what a
  person reads, and `matches` names the candidates when an id matched several.
- Only the frontend's own origins (`http://127.0.0.1:19428` and `http://localhost:19428`, from `.env`) may read an
  answer. Binding to loopback doesn't stop another page in the browser from calling the API, so the allowlist does. A
  preflight from one of those origins is answered with `204`; from anywhere else it isn't.

## `GET /api/sessions`

Every session under the transcript root, newest first. Reads directory entries and the two ends of each transcript, so
it costs about 140 ms over 722 sessions and 3.5 GB rather than parsing any of it.

- `?limit=N` caps the page, and `0` or no limit gives everything. `totals` counts every session either way.

```json
{
    "root": "/Users/you/.claude/projects",
    "sessions": [
        {
            "id": "532ac591-b7c5-45ca-a764-f40f01a0a9ac",
            "projectSlug": "-Users-you-projects-git-vdavid-cmdr",
            "projectPath": "/Users/you/projects-git/vdavid/cmdr",
            "projectName": "cmdr",
            "title": "Make search and select work unindexed",
            "start": "2026-08-03T08:42:19.17Z",
            "end": "2026-08-06T09:51:26.407Z",
            "seconds": 263347.237,
            "modified": "2026-08-06T09:52:01.409Z",
            "subagents": 27,
            "bytes": 66984138
        }
    ],
    "totals": { "sessions": 722, "subagents": 3258, "bytes": 3468649374 }
}
```

- `start` and `end` are the **lead** transcript's first and last timestamped records. A subagent can outlive the lead,
  so a timeline's own span can run wider; see `totals.from` below.
- `subagents` counts the lanes the session spawned, the ones its workflows spawned included. The lead isn't one of them,
  so this is one less than a timeline's `totals.lanes`.
- `title` is empty for a session that never got one, and `start` is `null` for one whose records carry no timestamps.
  `modified` is always there.

## `GET /api/sessions/{id}`

One session's metadata, for a session page header. `{"session": {…}}`, the same object the list holds.

## `GET /api/sessions/{id}/timeline`

The rows, plus everything a chart needs already summed. Aggregating here rather than in the browser is the whole point:
a session's 15,944 rows are a Go loop, not a JavaScript one.

- `?rows=false` leaves the rows out and returns the aggregates alone. That's 373 KB instead of 8.1 MB on the 983-lane
  session, and `totals.rows` still says how many there were.

```json
{
    "session": { "…": "the object above" },
    "totals": {
        "from": "2026-08-03T08:42:19.17Z",
        "until": "2026-08-06T10:11:03.289Z",
        "wallClockSeconds": 264524.119,
        "laneTimeSeconds": 405702.456,
        "rows": 15944,
        "lanes": 28,
        "byKind": [{ "kind": "thinking", "seconds": 29450.346, "rows": 2300 }],
        "byTool": [
            {
                "group": "codegraph (MCP)",
                "class": "mcp",
                "calls": 4,
                "seconds": 3.427,
                "lanes": 2,
                "tools": [
                    {
                        "tool": "mcp__codegraph__codegraph_explore",
                        "leaf": "codegraph_explore",
                        "calls": 2,
                        "seconds": 1.492,
                        "lanes": 1
                    }
                ]
            }
        ]
    },
    "lanes": [
        {
            "id": "am0-scope-ceiling-7aba8978eb07ad55",
            "name": "m0-scope-ceiling",
            "isLead": false,
            "model": "claude-opus-5",
            "color": "cyan",
            "from": "2026-08-04T05:44:33.822Z",
            "until": "2026-08-04T07:16:00.517Z",
            "seconds": 5486.695,
            "rows": 715,
            "byKind": [{ "kind": "thinking", "seconds": 864.749, "rows": 84 }],
            "gaps": [
                {
                    "from": "2026-08-03T08:42:19.17Z",
                    "until": "2026-08-03T08:44:16.703Z",
                    "seconds": 117.533,
                    "kind": "waiting for a person",
                    "info": "waiting for the next prompt"
                }
            ]
        }
    ],
    "rows": [
        {
            "from": "2026-08-03T08:44:16.703Z",
            "until": "2026-08-03T08:44:18.7Z",
            "seconds": 1.997,
            "laneId": "532ac591-b7c5-45ca-a764-f40f01a0a9ac",
            "agent": "lead",
            "kind": "writing",
            "info": "I'll dig into both. Let me start by finding the relevant code.",
            "line": 18
        },
        {
            "from": "2026-08-03T09:12:04.11Z",
            "until": "2026-08-03T09:14:58.902Z",
            "seconds": 174.792,
            "laneId": "am1-engine-aeeff1f0",
            "agent": "m1-engine",
            "kind": "tool execution",
            "info": "Bash (test): go test ./...",
            "tool": "Bash",
            "class": "test",
            "toolGroup": "Bash (test)",
            "line": 512
        }
    ]
}
```

### The two numbers that aren't the same

`wallClockSeconds` is how long the session took. `laneTimeSeconds` is every lane's rows added up, which is larger
whenever lanes ran at the same time: 405,702 s against 264,524 s on the reference session. They answer different
questions, and presenting one as the other is the mistake this API is shaped to prevent. A pie of `byKind` is a
breakdown of **lane time**, so say so in the legend.

`totals.from` and `totals.until` bracket every lane, so they can sit slightly outside the session's own `start` and
`end`, which are the lead's alone. On the reference session a subagent outlives the lead by 20 minutes. Both are `null`
on a session whose records carry no timestamp, which is 99 of the 725 on this machine, and `totals.rows` is `0` there: a
consumer that draws a timeline has nothing to draw.

### The tool breakdown

`totals.byTool` answers "which tools did this session use, and who used them". One entry per **group**, most calls
first, each holding the exact tools inside it.

- **A group is a tool at the level a reader asks about.** The raw name is useless as a grouping: `Bash` is 62% of every
  call in the corpus and an MCP server arrives as one tool per method, so a breakdown by name is three big slices and a
  smear (sampled 2026-08-06 over 76,708 calls in 624 transcripts). So `Bash` splits by what the command was doing
  (`Bash (git)`, `Bash (checker)`) and a server's methods collapse into the server (`codegraph (MCP)`). The rule is
  `timeline.Identify`, and `docs/timeline-rules.md` has it.
- **`leaf` is the exact thing inside the group**: an MCP method, or the program a `Bash` call ran (`git commit`,
  `cargo test`, `pnpm check`). `tool` beside it is the raw name the harness used, for grepping a transcript.
- **`calls` counts calls, not rows.** Every call leaves a `tool call` row for the agent composing it and one more row
  for the tool running, and only the second is counted. `seconds` is those second rows added up, so it's the tool's own
  wall clock including anything the tool waited on, and parallel calls each count their own the way lane time does.
- **`lanes` is how many lanes made one of these calls.** It's counted per level rather than added up: one lane calling
  two of a server's methods is one lane for the server and one for each method, so a group's count is never the sum of
  its tools'.
- `errors` and `timedOut` are left out when they're zero. `class` is the same string a row carries, and every tool in a
  group shares it.
- Every row carrying a `tool` also carries `toolGroup`, so a consumer can filter to a slice's rows without re-deriving
  the grouping rule. The leaf isn't on the row: nothing filters by it, and the breakdown already carries the leaves.
- A session that never called a tool gets `[]`.

### Lanes and gaps

- **Group rows by `laneId`, never by `agent`.** Two lanes carry the same name when neither has a `.meta.json`, and one
  session on this machine has 977 lanes all called `workflow-subagent`.
- `workflowId` names the workflow that spawned a lane, and is absent for one the session spawned directly.
- A field that doesn't apply is left out rather than sent empty: `tool` and `class` only appear on a tool call, a tool
  execution, or a stall, and `overlapped`, `timedOut`, and `isError` only when they're true.
- `color` is what the terminal used and it's often missing, so the UI needs a palette of its own.
- `line` is a line number inside one transcript file, and a lane is occasionally written across several of them
  (`docs/transcript-format.md`), so two rows of the same lane can carry the same `line`. It's a pointer for tracing, not
  a key.
- `gaps` are the stretches a lane produced nothing: its waiting, `stalled`, and `API error` rows, in time order. A
  swimlane bar drawn solid from `from` to `until` would claim the lane was busy the whole time, and on the reference
  session's lead that would be a lie for 71 of its 73 hours. Each gap carries its own `kind`, so the four waits can be
  drawn apart.
- `byKind` lists only the kinds with rows behind them, in the order a legend should show them: `thinking`, `writing`,
  `tool call`, `tool execution`, `waiting for a person`, `waiting for a teammate`, `waiting for a background task`,
  `waiting, reason unknown`, `API error`, `stalled`, `compacting`. Waiting is four kinds rather than one, so a pie has a
  slice per thing waited on. What each one means, and what it includes: `docs/timeline-rules.md`.
- `rows` are sorted by start across every lane. `overlapped` marks the only rows that don't tile their lane: a batch of
  parallel tool calls genuinely ran at once.

## Status codes

- `200` with a body.
- `400` `bad_request` for a parameter that isn't a number, and `ambiguous_id` when an id matched several sessions. The
  body's `matches` names them.
- `404` `not_found` for an unknown id or an unknown path.
- `405` `method_not_allowed`, with an `Allow: GET` header.
- `500` `internal` for a transcript that couldn't be read, and `no_transcripts` when the root isn't there at all.

## What it costs

Measured on the machine this was built against (2026-08-06, 732 sessions, 3.8 GB):

- `/api/sessions`: 280 KB in 124 ms.
- The 28-lane reference session's timeline: 6.5 MB in 1.1 s, or 59 KB with `?rows=false`. `byTool` is 14 KB of that,
  over 27 groups.
- The 983-lane session's timeline: 8.1 MB in 1.9 s, or 373 KB with `?rows=false`.

No compression: this is loopback, where a megabyte costs a millisecond and the CPU spent zipping it wouldn't come back.
