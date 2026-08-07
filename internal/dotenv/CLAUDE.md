# dotenv/: the committed `.env` reader

Reads repo's `.env`, which holds two dev ports. A dozen lines rather than a dependency: `KEY=value` with comments and
optional quotes is all it must understand.

## Must-knows

- **Process environment wins over file.** `.env` is a default someone can override for one run, not something that
  overrides them.
- **Walks up from directory it's given**, so the binary runs from anywhere inside the repo and still finds ports.
- **Missing file is not an error.** `Load` returns empty `Env`, every lookup falls through to caller's fallback.
  `internal/cli/serve.go` holds those fallbacks, same numbers `.env` carries.
- Nothing secret here. File is committed so a fresh clone runs; anything that shouldn't be in git needs another home.

## Module map

- `dotenv.go`: `FileName`, `Load`, `Env.Get`, parser. Whole package.
