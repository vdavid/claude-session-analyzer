# transcript/: the on-disk format

Streams records off a session's JSONL, decodes ones this tool understands. Faithful to file, nothing more: judgement
about what file _means_ lives in `internal/timeline`.

## Must-knows

- **Format is reverse-engineered and drifts.** `docs/transcript-format.md` is source of truth, every claim carries how
  and when it was checked, and a change here means a change there.
- **Unknown record or block type: skip and count, never error.** New ones ship regularly. `Stats` lets a caller account
  for every line: `Decoded + Skipped + Blank + Malformed == Lines`.
- **Parse per block, not per record.** Older transcript packs whole response into one record, up to 13 blocks including
  11 parallel `tool_use` calls.
- **No `bufio.Scanner`.** Its 64 KB default fails on real data: longest line in corpus is 3.42 MB. Reader grows a
  `bufio.Reader` buffer, no ceiling.
- **Payloads truncated to `MaxValueBytes` (8 KB).** Enough for a command line and head of its output. Pass `Unlimited`
  for the whole thing, expect the memory.
- Non-message record often carries no `timestamp`. Zero time is normal, never 1970.

## Module map

- `reader.go`: `Reader` (`Next` / `Record` / `Err` / `Stats`), growing line reader, `Options`.
- `record.go`: `Record`, `Block`, typed payloads (`APIError`, `Usage`, `SystemInfo`, `CompactInfo`, `QueueInfo`).
