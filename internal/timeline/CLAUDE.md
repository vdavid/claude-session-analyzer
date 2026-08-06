# timeline/: lanes into activity rows

Where the judgement lives, and where being wrong is invisible in the output. Every rule and the evidence behind it:
`docs/timeline-rules.md`. Read it before changing one, and update it when you do.

## Must-knows

- **Rows tile their lane.** Each starts where the last ended, the first at the lane's start, the last at its end. The
  only exception is a batch of parallel tool calls, and each of those rows says `Overlapped`. Break the tiling and every
  total downstream stops adding up.
- **The cursor only moves forward.** Timestamps go backwards (write jitter, and a 132 s jump at compaction), so a record
  stamped before the cursor gets a zero-length row rather than a negative one.
- **Group by `LaneID`, never by `Agent`.** 23 lanes in one session on this machine are all called `general-purpose`.
- **Ask `Kind.IsWaiting()` and `Kind.IsGap()`, don't list kinds.** A new kind added to a hand-written list somewhere
  else is a bug nobody sees.
- **A rule is test-first here.** Write the failing case, watch it fail for the right reason, then implement.
- **A threshold carries its distribution next to the constant** (`options.go`, `stall.go`). A number with no measurement
  behind it is a number the next person can't argue with.
- The golden CSV holds the whole derivation. Rewrite it with `go test ./internal/timeline -update` and read the diff
  before committing it.

## Module map

- `derive.go`: the walk. `kind.go`: the 11 kinds and the two predicates. `options.go`: the two spans that bound a
  heuristic.
- `wait.go` (what ended a gap, and who was alive), `stall.go` (late results and timeouts), `apierror.go` (a refused
  request), `tool.go` (what a call was doing), `info.go` (the `Extra info` column), `csv.go` (the CSV contract).

Three tests derive this machine's own sessions and skip by default (`CSA_REAL_SESSION_ID`, `CSA_SWEEP`). Run them after
changing a rule: the sweep checks the tiling on every session and prints every stall it called. Commands: `AGENTS.md` §
Checks.
