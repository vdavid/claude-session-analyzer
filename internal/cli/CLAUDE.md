# cli/: the command line surface

`sessions`, `timeline`, and `serve`, over the engine. It parses arguments and renders; anything involving a judgement
call belongs in `internal/timeline`, not here.

## Must-knows

- **`Run(args, stdout, stderr) int` is the whole surface**, so every command is tested without a process. Keep it that
  way: nothing here reaches for `os.Stdout` or `os.Exit`.
- **Exit codes mean something.** 0 worked, 1 the command couldn't do its job, 2 the command line didn't make sense
  (`usageError`). A script tells "you asked wrong" from "it didn't work" by those.
- **Use `parseArgs`, never `flag.Parse` directly.** The flag package stops at the first positional argument, which would
  break `timeline <id> --out file.csv`. `parseArgs` sets the argument aside and carries on, so flags work on either side
  of the id.
- **Messages are sentences with a next step in them**, and they never say "error" or "failed". `resolve.go` holds the
  two that matter: an unknown id and an ambiguous one.
- **`serve` binds `127.0.0.1` and only that.** Ports come from the committed `.env` (`internal/dotenv`), a flag beats
  the file, and the frontend's port is read only to name the origins the browser will let read an answer.
- `formatBytes` here and `formatBytes` in `web/src/lib/format.ts` have to agree: the same number shows up on both
  surfaces.

## Module map

- `cli.go`: `Run`, the subcommand table, usage, `parseArgs`.
- `sessions.go`, `timeline.go`, `serve.go`: one per subcommand. `resolve.go`: id and root resolution, and the messages
  for both.
- `format.go`: terminal rendering (clipping, thousands separators, human bytes). JSON keeps the raw numbers.
