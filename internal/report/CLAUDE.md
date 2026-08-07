# report/: the JSON both surfaces answer with

Shapes and the rendering from engine types. HTTP API and the CLI's `--json` return these same structs from this same
code, so a caller learns one vocabulary. Contract: `docs/api.md`. Changing a tag here means changing that doc.

## Must-knows

- **A ladder plus wall clock, never interchangeable.** `laneTimeSeconds` is every lane's rows added up (larger whenever
  lanes ran at once), `netSeconds` is that minus `Kind.IsSomeoneElsesClock()` (waiting on a person or a teammate),
  `activeSeconds` is net minus the rest of `Kind.IsGap()`. Each rung is the one above minus something, so they're never
  rivals. `wallClockSeconds` is a different axis: how long the session took. Presenting one as another is the mistake
  this package is shaped to prevent. Definition and arithmetic: `docs/api.md`.
- **`byKind` is a breakdown of lane time.** So is a group's share anywhere downstream: the kinds partition lane time and
  nothing else.
- **A tool group reports three clocks, not one.** `seconds` is the tool running, `composingSeconds` is the agent writing
  the calls, `stalledSeconds` is a call that came back far too late. All three carry the group's name, so together they
  account for every row the grouping rule put in the group, and `report_test.go` holds them to that. A stall is still
  one of `calls`.
- **An unknown instant is `null`, never a zero date.** 99 of the 725 sessions on this machine carry no timestamped
  record, and `0001-01-01` reaches a reader as a real date.
- **Totals come from `internal/agg`, not a hand-rolled tally.** `TotalsFrom` takes cells, so a cached digest and a live
  parse can't disagree. `ToolTotals` splits them with `agg.ToolRuns`, `agg.Composing`, and `agg.Stalls`: a tool's own
  clock excludes both the row where the agent composed the call and the run that stalled.
- **Gaps and rows are the one pass over the rows**, because a cube can't hold either.
- **A field that doesn't apply is left out, not sent empty**, so `tool`, `class`, `toolGroup`, `overlapped`, `timedOut`,
  and `isError` only appear where they mean something.
- `Seconds` rounds to the millisecond transcripts are stamped at, and it's the only place rounding happens.

## Module map

- `report.go`: every JSON struct, `ForSession`, `ForTimeline`, and the three reusable pieces a `stats` answer shares:
  `TotalsFrom`, `KindTotals`, `ToolTotals`.
- `report_test.go`: `TestTheLadderHoldsFromLaneTimeDownToActive` and
  `TestAToolGroupsThreeClocksAccountForEveryRowCarryingItsName` are the guards on five confusable durations. They run
  over a hand-built timeline holding a row of every kind, so a twelfth kind fails the test until someone decides which
  rung it belongs to. Don't weaken them; add to `ladderLengths` instead.
