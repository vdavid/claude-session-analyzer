# timeline/: lanes into activity rows

Where judgement lives, and where being wrong is invisible in output. Every rule and its evidence:
`docs/timeline-rules.md`. Read before changing a rule, update when you do.

## Must-knows

- **Rows tile their lane.** Each starts where last ended, first at lane's start, last at its end. Only exception: a
  batch of parallel tool calls, each of those rows says `Overlapped`. Break tiling and every total downstream stops
  adding up.
- **Cursor only moves forward.** Timestamps go backwards (write jitter, and a 132 s jump at compaction), so a record
  stamped before cursor gets a zero-length row, not a negative one.
- **Group by `LaneID`, never by `Agent`.** 23 lanes in one session on this machine are all called `general-purpose`.
- **Ask `Kind.IsWaiting()`, `Kind.IsGap()`, `Kind.IsSomeoneElsesClock()`, `Row.IsToolRun()`, don't list kinds.** A new
  kind added to a hand-written list elsewhere is a bug nobody sees. `IsToolRun` is the one row per call holding the
  tool's own clock, whatever verdict it got; its `tool call` sibling is the agent composing the call, and counting both
  reports every call twice. `IsSomeoneElsesClock` is the two waits net time takes out (a person's, a teammate's), and
  the reason it's a predicate is in its doc comment.
- **Every class a command can be read as has to be in `precedence`** (`tool.go`). One that isn't can never outrank the
  `ClassShell` a command starts out as, so it gets mapped and then never returned. `ClassWeb` sat outside it once.
- **A wait ended by a task notification asks whose task it was.** A live other lane's task makes it
  `waiting for a teammate`; the lane's own, a finished lane's, or an id no lane claims stays
  `waiting for a background task`. `attributeTaskWaits` (`wait.go`) runs as a post-pass in `Derive`, after the lane
  spans exist and before `nameWaits`, and before the sort, which is what keeps the row indices the walk collected valid.
  The tool-use id travels in a `map[int]string` from row index, never on `Row`: nothing downstream should read a
  tool-use id off a wait.
- **A rule is test-first here.** Write failing case, watch it fail for the right reason, then implement.
- **A threshold carries its distribution next to the constant** (`options.go`, `stall.go`). A number with no measurement
  behind it is one the next person can't argue with.
- Golden CSV holds the whole derivation. Rewrite with `go test ./internal/timeline -update`, read diff before
  committing.

## Module map

- `derive.go`: the walk. `kind.go`: 11 kinds, three predicates. `options.go`: two spans bounding a heuristic.
- `wait.go` (what ended a gap, who was alive), `stall.go` (late results, timeouts), `apierror.go` (a refused request),
  `tool.go` (what a call was doing, two names `ToolID` gives it), `info.go` (the `Extra info` column), `csv.go` (CSV
  contract).
- `Identify` reads a call once, returns class plus both breakdown names. `Classify` is the thin wrapper for callers
  wanting only the class, so don't add a second parse of the same shell command.

Three tests derive this machine's own sessions, skip by default (`CSA_REAL_SESSION_ID`, `CSA_SWEEP`). Run after changing
a rule: sweep checks tiling on every session and prints every stall it called. Commands: `AGENTS.md` § Checks.
