# dotenv/: the committed `.env` reader

Reads the repo's `.env`, which holds the two dev ports. A dozen lines rather than a dependency, because `KEY=value` with
comments and optional quotes is all it has to understand.

## Must-knows

- **The process environment wins over the file.** A `.env` is a default someone can override for one run, not something
  that overrides them.
- **It walks up from the directory it's given**, which is what lets the binary run from anywhere inside the repo and
  still find the ports.
- **A missing file is not an error.** `Load` returns an empty `Env`, and every lookup falls through to the caller's
  fallback. `internal/cli/serve.go` holds those fallbacks, and they're the same numbers `.env` carries.
- Nothing secret goes in here. The file is committed so a fresh clone runs, and anything that shouldn't be in git needs
  a different home.

## Module map

- `dotenv.go`: `FileName`, `Load`, `Env.Get`, and the parser. That's the package.
