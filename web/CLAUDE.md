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
  activity kind or a tool category. Both vocabularies live in `src/app.css` and nowhere else, and no chart mixes them:
  each sits in its own section behind its own legend. Look a kind or category up by name, never by array index, because
  the API only sends what a session has.
- **The tool taxonomy is served, never defined here.** `toolCategories` on the timeline response carries the seven
  category names, their labels, and the order; every group carries its own `category`. `src/lib/categories.ts` holds
  only the name-to-CSS-variable palette. Never map a `class` onto a bucket here: that mapping lives in
  `internal/timeline/category.go` and has exactly one definition.
- **Tool palette is seven slots, and the API's category order is the safety mechanism.** It decides which pairs touch in
  the pie. Re-run the validator before changing a slot, and re-derive the order if the engine ever reorders
  `Categories`: exact two commands, the numbers, and the two failures that are by design are in `docs/frontend.md`.
- **A category the API grows falls to the neutral slot and sorts last.** `categoryVar` returns `--csa-tool-other` for a
  name with no slot, and `toolBreakdown` ranks an unlisted category after every listed one. Right failure for a page:
  the groups still show up, uncoloured rather than missing.
- **Never present a `byKind` share as a share of elapsed time.** It's lane time, which is larger. `docs/api.md` § The
  ladder.
- **A tool group's time is three numbers, and nothing here adds them into one.** `runningSeconds` (the API's `seconds`,
  renamed here so it can't read as a cost), `composingSeconds`, `stalledSeconds`. `attributedSeconds` is their sum and
  is named for what it is: every second filed under the group's name. It's a bar length and a sort key, never a printed
  figure, because one number over all three reports a tool as costing what the agent and a suspension cost. Three table
  columns, no total column, and every duration on screen says which clock it is.
- **The three clocks wear treatments of the category hue, not hues of their own.** `clock-fill`, `clock-fill-composing`,
  `clock-fill-stalled` in `app.css`, over a `--clock-hue` set inline from `categoryVar`. Two colour vocabularies is the
  budget; the encoding decision and why is `docs/frontend.md` § Three clocks, three treatments, not three hues.
- **Lane time, net agent time, and active time are a ladder, never three rivals.** `netSeconds` and `activeSeconds` come
  off the API; `timeLadder` measures the subtractions. Show the subtraction wherever a rung shows, and never put elapsed
  on the ladder.
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
  `categories.ts` (the tool-category palette: name to CSS variable, and the neutral an unknown name falls to),
  `theme.svelte.ts` (stylesheet read back out for canvas, following `prefers-color-scheme`; `chrome.decalInk` is the one
  definition of the ink a texture is drawn in, shared with the CSS bars).
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
