# web/: the local web app

SvelteKit with `adapter-static`, Svelte 5 runes, Tailwind v4. Two routes over the Go API, no server of its own. Contract
it consumes: `docs/api.md`. Read that before touching anything that reads a JSON field.

## Must-knows

- **Ports come from repo root `.env`**, read by `vite.config.ts` through `loadEnv` with `envDir: '..'`. Vite proxies
  `/api` to the backend port, so the app is same-origin with its API and carries no base URL. Don't hardcode a port,
  don't add a `VITE_API_URL`.
- **`ssr = false` and `prerender = false`** in `src/routes/+layout.ts`. Everything comes from a Go server on this
  machine, so nothing to render ahead of time.
- **Group by `laneId`, never by `agent`.** 977 lanes in one session share the name `workflow-subagent`.
- **Colour belongs to data.** Chrome is neutral, one blue for links and focus; every saturated pixel stands for an
  activity kind or a tool family. Both vocabularies live in `src/app.css` and nowhere else, and no chart mixes them:
  each sits in its own section behind its own legend. Look a kind or class up by name, never by array index, because the
  API only sends what a session has.
- **Tool palette is seven slots in a fixed order, and the order is the safety mechanism.** It decides which pairs touch
  in the pie. Don't reorder `FAMILY_ORDER` (`src/lib/classes.ts`), and re-run the validator before changing a slot:
  exact two commands, the numbers, and the two failures that are by design are in `docs/frontend.md`.
- **A tool class the engine grows falls silently into the neutral family.** `CLASS_FAMILIES` in `src/lib/classes.ts` is
  a hand-written map of the 15 classes in `internal/timeline/tool.go`; a name missing from it draws as "Everything else"
  rather than throwing. Right failure for a page, and the one that goes unnoticed: adding a `ToolClass` in Go means
  adding it there in the same change.
- **Never present a `byKind` share as a share of elapsed time.** It's lane time, which is larger. `docs/api.md` § The
  two numbers that aren't the same.
- Tailwind tokens are registered `@theme inline` over custom properties. An opacity modifier on one compiles to a
  `color-mix()` behind `@supports`, solid colour as fallback, which is why the sticky header is opaque rather than
  broken on a browser that can't mix.
- ECharts is assembled from parts in `components/charts/echarts.ts`. A new chart type has to register itself there.
- **Light ink ramp is set by its tightest pairing**, faint text on `sunken`, clearing 4.5:1 with nothing spare. Don't
  lighten `--csa-ink-faint` or `--csa-ink-muted`; both pages audit clean under axe in both themes.

## Module map

- `src/lib/`: `api.ts` (every call to the server, errors as `ApiError` with a `code` to branch on), `types.ts` (JSON
  shapes), `format.ts` (durations, instants, bytes; `formatBytes` matches `humanBytes` in `internal/cli/format.go`),
  `kinds.ts` (the eleven kinds: legend order, colour variable, family, and the sentence keeping each honest),
  `classes.ts` (engine's 15 tool classes mapped onto seven drawn families, in the order the palette was validated in),
  `theme.svelte.ts` (stylesheet read back out for canvas, following `prefers-color-scheme`).
- `src/lib/transform/`: the tested layer. API JSON into chart series. Nothing else is unit-tested. Must-knows:
  `web/src/lib/transform/CLAUDE.md`.
- `src/lib/components/`: `charts/` (one component per chart, all over `charts/Chart.svelte`), plus the sheet, the
  legend, the small display pieces. Must-knows: `web/src/lib/components/CLAUDE.md` and
  `web/src/lib/components/charts/CLAUDE.md`.
- `tests/`: Vitest over `src/lib/transform/` and `format.ts`.

## Checks

`pnpm check web` at repo root, or `pnpm check prettier` / `eslint` / `svelte-check` / `vitest` for one gate. A green
build is not evidence the page works: drive it with `pnpm dev` against a real session before calling anything done.

No automated accessibility or browser gate. When you touch layout or colour, run axe over both pages in both themes
yourself. **The `.rise` entrance animation moves and never fades**, because text composited mid-fade sits below AA
contrast for as long as the fade runs. Don't add an `opacity` keyframe back.

Deeper notes, including design system and what each chart is for: `docs/frontend.md`.
