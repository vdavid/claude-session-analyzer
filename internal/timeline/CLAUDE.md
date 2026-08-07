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
- **A class is what the work was for, not what it mechanically is.** `cargo check` compiles and is a `lint`, because it
  produces nothing; `cargo doc` renders HTML and is a `build`. Reasoning and the corpus numbers:
  `docs/timeline-rules.md`.
- **Adding a `ToolClass` costs four edits in the same change**: `Classes` and `precedence` (`tool.go`),
  `classCategories` (`category.go`), and `stallThreshold` (`stall.go`) if the work earns the generous line. All but
  `precedence` fail loudly (`TestTheClassListMatchesTheEngines` in `internal/stats`, `TestEveryClassHasACategory` here).
  Nothing in the frontend needs touching: it reads the taxonomy off the API.
- **`ToolCategory` (`category.go`) is configuration, not engine truth.** Seven buckets over the sixteen classes, and the
  per-group overrides encode one person's workflow semantics (`codegraph (MCP)` is reading, `WebFetch` is not QA). Keep
  it a flat reviewable list someone can change without reading the rest of this package, each override carrying its
  reason. A class decides the default; a group name overrides it. A row with no class gets no category, so thinking and
  waiting never land in one.
- **A taxonomy change invalidates every cached digest.** `ClassificationFingerprint` exists to be hashed by
  `internal/cache`'s `TestTheDigestVersionMovesWithTheDerivation`, which is what makes a mapping change fail with the
  `cache.Version` to bump. `classificationProbes` is what it covers, one representative call per class plus one per
  override, and a class no probe reaches is a rule change the guard can't see.
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
  `tool.go` (what a call was doing, `Classes`, two names `ToolID` gives it), `category.go` (the seven categories, the
  class defaults, the group overrides, `ClassificationFingerprint`), `info.go` (the `Extra info` column), `csv.go` (CSV
  contract).
- `Identify` reads a call once, returns class plus both breakdown names. `Classify` is the thin wrapper for callers
  wanting only the class, so don't add a second parse of the same shell command.

Three tests derive this machine's own sessions, skip by default (`CSA_REAL_SESSION_ID`, `CSA_SWEEP`). Run after changing
a rule: sweep checks tiling on every session and prints every stall it called. Commands: `AGENTS.md` § Checks.
