# cli/: the command line surface

`sessions`, `timeline`, `stats`, `cache`, and `serve`, over the engine. Parses arguments and renders; anything involving
a judgement call belongs in `internal/timeline`, `internal/agg`, or `internal/stats`, not here.

## Must-knows

- **`Run(args, stdout, stderr) int` is the whole surface**, so every command is tested without a process. Keep it that
  way: nothing here reaches for `os.Stdout` or `os.Exit`.
- **Exit codes mean something.** 0 worked, 1 the command couldn't do its job, 2 the command line didn't make sense
  (`usageError`). A script tells "you asked wrong" from "it didn't work" by those.
- **Use `parseArgs`, never `flag.Parse` directly.** The flag package stops at the first positional argument, which would
  break `timeline <id> --out file.csv`. `parseArgs` sets the argument aside and carries on, so flags work on either side
  of the id.
- **Bound what an agent reads.** `stats` defaults to `--top 20`, `sessions` to `--limit 20`, and `timeline --json`
  leaves rows out unless `--rows` asks. An 8 MB answer is a context-window incident, and these defaults are the guard.
- **`--json` answers with `internal/report` shapes**, the same ones the HTTP API returns. One vocabulary across both
  surfaces. `stats` is the exception: it has its own shape, `internal/stats`, documented in `docs/stats.md`.
- **A `stats` table shares against lane time, not active time.** Groups partition lane time, so an unfiltered column
  adds to 100%. Against active time a waiting group reads as 94% of a total it isn't part of. The JSON carries all three
  shares; the table names its denominator.
- **Messages are sentences with a next step in them**, and they never say "error" or "failed". `resolve.go` holds the
  two that matter: an unknown id and an ambiguous one.
- **`serve` binds `127.0.0.1` and only that.** Ports come from the committed `.env` (`internal/dotenv`), a flag beats
  the file, and the frontend's port is read only to name the origins the browser will let read an answer.
- **Progress goes to stderr**, so `--json` redirected to a file gets the answer alone.
- `humanBytes` here and `formatBytes` in `web/src/lib/format.ts` have to agree, down to the 1000-unit base: the same
  number shows up on both surfaces.

## Module map

- `cli.go`: `Run`, the subcommand table, usage, `parseArgs`.
- One file per subcommand: `sessions.go`, `timeline.go`, `stats.go`, `cache.go`, `serve.go`.
- `filter.go`: `--since`, `--until`, `--project`, shared by `sessions`, `stats`, and `cache warm`. A date-only `--until`
  covers the whole day, so `--since 2026-07-01 --until 2026-07-31` is the month a person means.
- `resolve.go`: id and root resolution, and the messages for both. `format.go`: terminal rendering (clipping, thousands
  separators, human bytes). `json.go`: the one `--json` writer.
