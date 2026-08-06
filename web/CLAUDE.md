# web/: the local web app

SvelteKit with `adapter-static`, Svelte 5 runes, Tailwind v4. Two routes over the Go API, no server of its own. The
contract it consumes is `docs/api.md`; read that before touching anything that reads a JSON field.

## Must-knows

- **Ports come from the repo root `.env`**, read by `vite.config.ts` through `loadEnv` with `envDir: '..'`. Vite proxies
  `/api` to the backend port, so the app is same-origin with its API and carries no base URL. Don't hardcode a port and
  don't add a `VITE_API_URL`.
- **`ssr = false` and `prerender = false`** in `src/routes/+layout.ts`. Everything comes from a Go server on this
  machine, so there's nothing to render ahead of time.
- **Group by `laneId`, never by `agent`.** 977 lanes in one session share the name `workflow-subagent`.
- **Colour belongs to data.** The chrome is neutral, with one blue for links and focus; every saturated pixel stands for
  an activity kind. Kind colours live in `src/app.css` and nowhere else. Look a kind up by name, never by array index:
  the API only sends kinds a session has rows for.
- **Never present a `byKind` share as a share of elapsed time.** It's lane time, which is larger. `docs/api.md` § The
  two numbers that aren't the same.
- Tailwind tokens are registered `@theme inline` over custom properties, so **opacity modifiers (`bg-canvas/50`) don't
  work** on them. Use `color-mix()`.
- ECharts is assembled from parts in `components/charts/echarts.ts`. A new chart type has to register itself there.
- **The light ink ramp is set by its tightest pairing**, faint text on `sunken`, which clears 4.5:1 with nothing spare.
  Don't lighten `--csa-ink-faint` or `--csa-ink-muted`; both pages audit clean under axe in both themes.

## Module map

- `src/lib/`: `api.ts` (every call to the server, errors as `ApiError` with a `code` to branch on), `types.ts` (the JSON
  shapes), `format.ts` (durations, instants, bytes; `formatBytes` matches `humanBytes` in `internal/cli/format.go`),
  `kinds.ts` (the eleven kinds: legend order, colour variable, family, and the sentence that keeps each honest),
  `theme.svelte.ts` (the stylesheet read back out for canvas, following `prefers-color-scheme`).
- `src/lib/transform/`: the tested layer. API JSON into chart series. Nothing else is unit-tested. Must-knows:
  `web/src/lib/transform/CLAUDE.md`.
- `src/lib/components/`: `charts/` (one component per chart, all over `charts/Chart.svelte`), plus the sheet, the
  legend, and the small display pieces. Must-knows: `web/src/lib/components/CLAUDE.md` and
  `web/src/lib/components/charts/CLAUDE.md`.
- `tests/`: Vitest over `src/lib/transform/` and `format.ts`.

## Checks

`pnpm check web` at the repo root, or `pnpm check prettier` / `eslint` / `svelte-check` / `vitest` for one gate. A green
build is not evidence the page works: drive it with `pnpm dev` against a real session before calling anything done.

There's no automated accessibility or browser gate. When you touch layout or colour, run axe over both pages in both
themes yourself. **The `.rise` entrance animation moves and never fades**, because text composited mid-fade sits below
AA contrast for as long as the fade runs. Don't add an `opacity` keyframe back.

Deeper notes, including the design system and what each chart is for: `docs/frontend.md`.
