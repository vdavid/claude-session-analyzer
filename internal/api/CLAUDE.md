# api/: the HTTP surface

Three endpoints over the engine, pie and swimlane already summed. Shapes live in `internal/report`, shared with the
CLI's `--json`; this package is transport only. Contract the web app is built against: `docs/api.md`. Changing a field
in `internal/report` means changing that doc.

## Must-knows

- **Aggregate here, not in the browser.** A session's 15,944 rows are a Go loop. Shipping them to be summed in
  JavaScript is the wrong split, and `?rows=false` exists so a page can have aggregates without the 7.7 MB.
- **Unknown instant is `null`, never a zero date.** Timestamps are `*time.Time` for exactly that reason: 99 of the 725
  sessions on this machine carry no timestamped record, and `0001-01-01` reached a reader once already.
  `TestATimelineWithNoTimestampsAnswersNullRatherThanYearOne` holds it.
- **CORS allowlist is load-bearing, a wildcard would be a hole.** Binding to loopback doesn't stop another page in the
  browser calling the API and reading every transcript on the machine. Only the frontend's two origins get an answer.
- **Every response is JSON, including failures.** Method-less route patterns exist so a `POST` gets a JSON `405` rather
  than the mux's plain text. Errors are `{"error": {"code", "message", "matches"}}`, and `code` is what a caller
  branches on.
- **A field that doesn't apply is left out, not sent empty**, so `tool`, `class`, `toolGroup`, `overlapped`, `timedOut`,
  and `isError` only appear where they mean something.
- **`byTool` counts calls, not rows, and its lane counts don't add up on purpose.** Only the row a call ran in is
  counted (`agg.ToolRuns`), because the `tool call` row beside it is the agent composing the call. A group keeps its own
  set of lanes rather than summing its tools': one lane calling two of a server's methods is still one lane.
- **This surface reads no cache.** `internal/cache` exists for corpus queries; a transcript grows while the server runs,
  and a page showing a stale timeline would be worse than a page taking a second.

## Module map

- `api.go`: `New`, routes, CORS, error bodies.
- `shapes.go`: the two HTTP envelopes and the error body. Everything else is `internal/report`.
