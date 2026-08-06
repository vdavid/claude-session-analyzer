# api/: the HTTP surface

Three endpoints over the engine, with the pie and the swimlane already summed. The contract the web app is built against
is `docs/api.md`; changing a field here means changing that doc.

## Must-knows

- **Aggregate here, not in the browser.** A session's 15,944 rows are a Go loop. Shipping them to be summed in
  JavaScript is the wrong split, and `?rows=false` exists so a page can have the aggregates without the 7.7 MB.
- **An unknown instant is `null`, never a zero date.** Timestamps are `*time.Time` for exactly that reason: 99 of the
  725 sessions on this machine carry no timestamped record, and `0001-01-01` reached a reader once already.
  `TestATimelineWithNoTimestampsAnswersNullRatherThanYearOne` holds it.
- **The CORS allowlist is load-bearing, and a wildcard would be a hole.** Binding to loopback doesn't stop another page
  in the browser from calling the API and reading every transcript on the machine. Only the frontend's two origins get
  an answer.
- **Every response is JSON, including the failures.** The method-less route patterns exist so a `POST` gets a JSON `405`
  rather than the mux's plain text. Errors are `{"error": {"code", "message", "matches"}}`, and `code` is what a caller
  branches on.
- **A field that doesn't apply is left out, not sent empty**, so `tool`, `class`, `overlapped`, `timedOut`, and
  `isError` only appear where they mean something.
- Nothing is cached. A transcript grows while the tool runs, so a cache would be another thing to invalidate.

## Module map

- `api.go`: `New`, the routes, CORS, and the error bodies.
- `shapes.go`: every JSON struct, and the rendering from engine types. The tags here are the contract.
