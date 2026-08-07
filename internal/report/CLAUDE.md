# report/: the JSON both surfaces answer with

Shapes and the rendering from engine types. HTTP API and the CLI's `--json` return these same structs from this same
code, so a caller learns one vocabulary. Contract: `docs/api.md`. Changing a tag here means changing that doc.

## Must-knows

- **Three totals, never interchangeable.** `wallClockSeconds` is how long the session took, `laneTimeSeconds` is every
  lane's rows added up (larger whenever lanes ran at once), `activeSeconds` is lane time minus every `Kind.IsGap()`.
  Presenting one as another is the mistake this package is shaped to prevent. `byKind` is a breakdown of lane time.
- **An unknown instant is `null`, never a zero date.** 99 of the 725 sessions on this machine carry no timestamped
  record, and `0001-01-01` reaches a reader as a real date.
- **Totals come from `internal/agg`, not a hand-rolled tally.** `TotalsFrom` takes cells, so a cached digest and a live
  parse can't disagree. `ToolTotals` filters through `agg.ToolRuns` first: a tool's own clock excludes the row where the
  agent composed the call.
- **Gaps and rows are the one pass over the rows**, because a cube can't hold either.
- **A field that doesn't apply is left out, not sent empty**, so `tool`, `class`, `toolGroup`, `overlapped`, `timedOut`,
  and `isError` only appear where they mean something.
- `Seconds` rounds to the millisecond transcripts are stamped at, and it's the only place rounding happens.

## Module map

- `report.go`: every JSON struct, `ForSession`, `ForTimeline`, and the three reusable pieces a `stats` answer shares:
  `TotalsFrom`, `KindTotals`, `ToolTotals`.
