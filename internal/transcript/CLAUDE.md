# transcript/: the on-disk format

Streams records off a session's JSONL and decodes the ones this tool understands. Faithful to the file, and nothing
more: judgement about what the file _means_ lives in `internal/timeline`.

## Must-knows

- **The format is reverse-engineered and drifts.** `docs/transcript-format.md` is the source of truth, every claim in it
  carries how and when it was checked, and a change here means a change there.
- **An unknown record or block type is skipped and counted, never an error.** New ones ship regularly. `Stats` lets a
  caller account for every line: `Decoded + Skipped + Blank + Malformed == Lines`.
- **Parse per block, not per record.** An older transcript packs a whole response into one record, up to 13 blocks
  including 11 parallel `tool_use` calls.
- **Don't reach for `bufio.Scanner`.** Its 64 KB default fails on real data: the longest line in the corpus is 3.42 MB.
  The reader grows a `bufio.Reader` buffer with no ceiling.
- **Payloads are truncated to `MaxValueBytes` (8 KB).** Enough for a command line and the head of its output. Pass
  `Unlimited` when you need the whole thing, and expect the memory.
- A record that isn't a message often carries no `timestamp`. A zero time is normal, and never means 1970.

## Module map

- `reader.go`: `Reader` (`Next` / `Record` / `Err` / `Stats`), the growing line reader, and `Options`.
- `record.go`: `Record`, `Block`, and the typed payloads (`APIError`, `Usage`, `SystemInfo`, `CompactInfo`,
  `QueueInfo`).
