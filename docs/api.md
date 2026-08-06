# The HTTP API

What `claude-session-analyzer serve` answers, and what the web app is built against. `internal/api` renders it;
changing a field there means changing this.

The server binds to `127.0.0.1` and nothing else. A tool that reads every transcript on the machine has no business on
the network, so the address isn't configurable.

## Conventions

- Every instant is RFC 3339 in UTC. A timestamp that isn't known is `null`, never a zero date: 98 of the 722 sessions
  on the machine this was built against carry no timestamped record at all.
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
- `subagents` counts the lanes the session spawned, the ones its workflows spawned included. The lead isn't one of
  them, so this is one less than a timeline's `totals.lanes`.
- `title` is empty for a session that never got one, and `start` is `null` for one whose records carry no timestamps.
  `modified` is always there.

## `GET /api/sessions/{id}`

One session's metadata, for a session page header. `{"session": {…}}`, the same object the list holds.

## `GET /api/sessions/{id}/timeline`

The rows, plus everything a chart needs already summed. Aggregating here rather than in the browser is the whole point:
a session's 15,944 rows are a Go loop, not a JavaScript one.

- `?rows=false` leaves the rows out and returns the aggregates alone. That's 364 KB instead of 7.7 MB on the 983-lane
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
    "byKind": [{ "kind": "thinking", "seconds": 29450.346, "rows": 2300 }]
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
`end`, which are the lead's alone. On the reference session a subagent outlives the lead by 20 minutes.

### Lanes and gaps

- **Group rows by `laneId`, never by `agent`.** Two lanes carry the same name when neither has a `.meta.json`, and one
  session on this machine has 977 lanes all called `workflow-subagent`.
- `workflowId` names the workflow that spawned a lane, and is absent for one the session spawned directly.
- A field that doesn't apply is left out rather than sent empty: `tool` and `class` only appear on a tool call, a tool
  execution, or a stall, and `overlapped`, `timedOut`, and `isError` only when they're true.
- `color` is what the terminal used and it's often missing, so the UI needs a palette of its own.
- `gaps` are the stretches a lane produced nothing: its waiting, `stalled`, and `API error` rows, in time order. A swimlane bar drawn
  solid from `from` to `until` would claim the lane was busy the whole time, and on the reference session's lead that
  would be a lie for 71 of its 73 hours. Each gap carries its own `kind`, so the four waits can be drawn apart.
- `byKind` lists only the kinds with rows behind them, in the order a legend should show them: `thinking`, `writing`,
  `tool call`, `tool execution`, `waiting for a person`, `waiting for a teammate`, `waiting for a background task`,
  `waiting, reason unknown`, `API error`, `stalled`, `compacting`. Waiting is four kinds rather than one, so a pie has
  a slice per thing waited on. What each one means, and what it includes: `docs/timeline-rules.md`.
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

Measured on the machine this was built against (2026-08-06, 722 sessions, 3.5 GB):

- `/api/sessions`: 273 KB in 140 ms.
- The 28-lane reference session's timeline: 5.5 MB in 0.7 s, or 40 KB with `?rows=false`.
- The 983-lane session's timeline: 7.7 MB in 1.6 s, or 364 KB with `?rows=false`.

No compression: this is loopback, where a megabyte costs a millisecond and the CPU spent zipping it wouldn't come back.
