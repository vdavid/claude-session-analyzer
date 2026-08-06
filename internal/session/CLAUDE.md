# session/: finding sessions and their lanes

Turns a session id into files on disk, and files into lanes: the lead plus one per subagent, each holding its records in
order. Layout facts and their evidence: `docs/transcript-format.md`.

## Must-knows

- **A lane is its path inside the session directory, not the file it sits in.** A session that entered a worktree writes
  `<session-id>/subagents/` under the worktree's slug, so one lane arrives as fragments under several slugs. `Load`
  merges them by timestamp; concatenating them in slug order breaks the `parentUuid` chain.
- **A directory belongs to a session by its name, not by the slug around it.** `Find` scans every slug for a directory
  named after the id. Reading only the one beside the lead transcript missed 452 lane files on this machine.
- **`.meta.json` is often missing or partial**, and only one fragment of a split lane carries it. Labels fall back
  `name` → `agentType` → agent id, so two lanes can end up sharing a name. Key on `Lane.ID`.
- **A subagent file name can't be parsed.** `agent-aplan-reviewer-2-<hash>` is ambiguous by construction.
- **`List` must stay cheap.** It reads a 16 KB head and a 64 KB tail per session, growing either window only when what
  it's after isn't in it, which is what keeps 722 sessions and 3.5 GB at 150 ms. A change here shows up in
  `TestSummarizeReadsOnlyTheTwoEndsOfATranscript`, which counts the bytes through a wrapped `io.ReaderAt`.
- `Summary.Start` and `End` are the **lead's**. A subagent can outlive it, so a timeline's span can run wider.

## Module map

- `discover.go`: `DefaultRoot`, `Find` (id or unique prefix), `Location`, `LaneFiles`.
- `load.go` and `lane.go`: `Load` into a `Session` of `Lane` values, plus `AgentMeta` and the label fallback.
- `list.go`: `List` and `Summarize`, the cheap two-ends listing.

Three tests read this machine's own transcripts and skip by default (`CSA_REAL_SESSION_ID`, `CSA_REAL_LIST`,
`CSA_SWEEP`). Run them after touching decoding: hand-written fixtures can't catch format drift. Commands: `AGENTS.md` §
Checks.
