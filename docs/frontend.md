# The web app

What the two pages show, the design system behind them, decisions that shaped both. `web/CLAUDE.md` holds must-knows for
editing; this is depth.

## The two pages

**`/`** says what the tool is in three sentences, puts corpus totals beside them, lists every session on this machine.
One `GET /api/sessions` brings every one in a few hundred kilobytes, so sorting and filtering happen in browser: no
round trip, no pagination. Row carries title, project, id prefix, start, elapsed time, subagent count, transcript size;
header row sorts on any of those. Under the list, a collapsed "What these numbers don't say" note holds four caveats a
reader needs before trusting anything: thinking spans include model latency, lane time isn't elapsed time, stall
detection is a heuristic, thinking row's subject is borrowed from what came next.

**`/session/[id]`** analyses one session on load, in six stacked sections:

1. **The trace.** Stepped area strip under header, how many agents were producing at once across the span, peak called
   out. Shape of the session in one line: where parallel waves were, how much of the span had nobody working.
2. **The stat rail.** Elapsed, lane time, lanes, rows, each with its caveat printed under it rather than hidden in a
   tooltip. A number whose caveat lives elsewhere gets read wrong.
3. **Where lane time went.** Band bar (working / waiting / lost / compacting), then the donut with its legend as a table
   beside it. Hovering a legend row lights its slice.
4. **Who was alive, and when.** Swimlane, chip per workflow above it.
5. **What the agents reached for.** Tool donut, strip of category shares over it, legend as a table beside it. Picking a
   slice or legend row filters the sheet below to that tool's rows and scrolls to it.
6. **Every row.** The virtualized sheet.

## Decisions

### The API is reached through a Vite proxy, not by origin

`vite.config.ts` proxies `/api` to the backend port it reads out of repo root's `.env`, same file the Go server reads.
So page is same-origin with its API: no preflight, no base URL in the app, no failure when someone opens
`localhost:19428` instead of `127.0.0.1:19428`. Server's CORS allowlist (`docs/api.md`) still guards direct calls from
another page in the browser, the case it exists for.

### Aggregates first, rows second

Session page fetches `?rows=false`, renders every chart off it, then asks for rows, an order of magnitude larger
(`docs/api.md` § What it costs holds the measurements, so they can't drift in two places). Asking for both at once would
have the server parsing the same transcript twice at once, delaying what the reader waits for. Sheet section says how
many rows it's fetching while it waits.

### Colour belongs to data

Chrome is ink on cool paper in light mode, ink on slate in dark, one blue for links and focus. Every other saturated
pixel stands for an activity kind, so a coloured button would read as a kind that isn't in the legend. Work is cool
(violet, blue, teal, green), the four waits warm (orange, gold, bronze, taupe), the two ways a session loses time red
and magenta, compaction slate. Makes "how much of this was waiting" answerable from across the room, the first question
anyone asks.

Waits also carry a diagonal hatch in the pie and the two trouble kinds carry dots: second channel for anyone who can't
lean on hue, and a reminder those slices aren't work.

Kind colours live in `web/src/app.css`, read back out by `theme.svelte.ts` for canvas, because ECharts needs literals.
Light and dark are two blocks in one file, switched by `prefers-color-scheme`: no class toggling, no stored preference,
no flash on load.

### There are exactly two colour vocabularies, and each has its own legend

Tool breakdown can't use kind colours: it counts calls by what they were, not stretches of time by what an agent was
doing. So it has a second set, same file, and the page never puts the two in one chart. Each sits in its own section
behind its own legend, chrome between them stays neutral, so neither reads as the other.

Engine reports 16 tool classes, more than any palette keeps apart, so it folds them into **seven categories** and the
chart colours by category. Not a rollup for its own sake: because the pie draws categories in a fixed order, each
category is one contiguous arc and the chart says "37% of this session's calls were reading" before a reader touches the
legend. Groups inside a category share its colour, told apart by the 2px surface ring between them, by the table beside
the chart, and by the highlight the two share.

**The taxonomy is served, not defined here.** `internal/timeline/category.go` owns the class-to-category mapping and the
per-group overrides; the timeline response carries `toolCategories` (names, labels, descriptions, order) and a
`category` on every group. `docs/api.md` § The two levels of "what kind of work was this" is the contract. The frontend
holds only `categories.ts`, which maps a category name onto a CSS custom property, because a name is data and a hex
value is design. An unknown name draws in the neutral and sorts last rather than throwing.

Order is the colourblind-safety mechanism rather than a mood: it decides which pairs ever touch. Seven slots validated
in both modes (2026-08-06), worst adjacent pair ΔE 8.7 light and 11.0 dark under protanopia and deuteranopia, 17.4 and
19.2 under normal vision, every slot over 3:1 on its surface. Seventh slot is the neutral everything-else fold, so it
sits below the chroma floor on purpose, and the two flags it draws are expected. **The order that was validated is the
API's**, so reordering `Categories` in Go means re-deriving this.

To re-run it, load the `dataviz` skill and run the `validate_palette.js` it ships, passing the slots in the API's
category order (`management`, `read`, `write`, `build`, `checks`, `qa`, `other`) and **repeating the first one at the
end**: a pie closes on itself, so the last slot touches the first, and a linear pairlist would miss that pair. The two
calls that produced the numbers above, run from the skill's own base directory:

```sh
# light, against `--csa-surface`
node scripts/validate_palette.js \
  "#b5308a,#2f6fc4,#2f7d4f,#7b4fd0,#10a094,#c9611b,#828b98,#b5308a" --mode light --surface "#ffffff"
# dark, against the dark `--csa-surface`
node scripts/validate_palette.js \
  "#cf5a9b,#4f8fd8,#3fa76a,#8a72e0,#1aa892,#d97430,#a8b1bd,#cf5a9b" --mode dark --surface "#161b23"
```

Two of its checks fail on the neutral by design: below the chroma floor, and in dark above the lightness band.
Everything else has to pass. Changing a slot means nudging its lightness and re-running until worst adjacent pair
clears; changing the _order_ means re-deriving it, which is what produced this one.

### Type is the system font, deliberately

No web fonts. The tool reads transcripts off this machine and runs on it, and the terminal these sessions ran in is its
own vernacular, so display face is the system UI face and data face the system monospace (SF Mono on a Mac). Personality
comes from treatment: tabular monospace numerals everywhere a number appears, uppercase letterspaced micro-labels for
structure, durations set as the page's structural typography. Also means the page looks right offline, which a local
tool should.

### The entrance animation moves and never fades

Sections slide 6px into place, staggered, so the page assembles rather than blinking. No fade in, because a browser
composites text at partial opacity against the canvas behind it: faint ink at 40% alpha measures around 2:1 against
paper, so a fade puts every word under AA for as long as it runs, and someone reading during that first half-second is
the person the contrast ratio is for. Transform costs nothing in contrast and reads the same. Off entirely under
`prefers-reduced-motion`.

### A thousand lanes

Largest session here has 983 lanes, 977 spawned by 12 workflows and every one named `workflow-subagent`. Three things
together make it readable:

- **Workflows collapse.** One row per workflow, bar = union of its members'. 983 lanes become 18 rows. A hole in a
  workflow row means every lane in it was quiet, worth seeing on its own.
- **Rows scroll rather than shrink.** Chart draws a fixed window of full-height rows (`MAX_VISIBLE_ROWS`) and moves a
  y-axis slider over the rest. A canvas 983 rows tall is not a thing a browser should be asked for.
- **An opened workflow is capped** at 150 lanes and says how many it held back, and its lanes are labelled by id prefix
  because their names are all the same.

### Sub-pixel segments are left alone

Swimlane bars come out of `busySegments`, and counts are small enough not to need coalescing: 100 segments across
reference session's 31 lanes, 1,014 across the 983-lane one, 310 in the largest workflow row after the union (measured
2026-08-06). If a future session pushes that into tens of thousands, coalescing segments below a pixel is the move, but
it would cost fidelity under zoom and nothing needs it yet.

### The sheet is virtualized, and its filters are split

TanStack Table owns sorting and text search; four exact-match dropdowns (activity, tool, class, lane) narrow rows before
the table sees them, because a dropdown doesn't need a filter row model. Table is rebuilt when its state changes rather
than mutated in place, which costs one pass over the rows and buys a component with no lifecycle subtleties in it.
Search box is debounced by 180 ms so a keystroke doesn't rebuild 22,000 rows.

Tool dropdown is bound to the page rather than owned by the sheet, so clicking a slice two sections up and picking one
from the dropdown are the same act. Picking a group shows both of its rows per call, composing and running, because
they're what the derivation produced and the sheet's job is to show the rows.

## Dependency notes

- **`@tanstack/table-core` 8, not 9.** Svelte adapter only exists for v9, two days old when this was built and therefore
  inside the three-day release-age window. Core is framework-agnostic and wiring it to runes is a few lines, which is
  what shadcn-svelte does regardless. When v9 clears the window, `@tanstack/svelte-table` is the upgrade.
- **`@tanstack/virtual-core` rather than `@tanstack/svelte-virtual`.** Svelte adapter wraps the core in stores; runes
  read the core directly in about fifteen lines.
- **`minimumReleaseAge` lives in `pnpm-workspace.yaml`**, not `.npmrc`. That's where pnpm 11 reads its settings, and npm
  rejects the key with a warning on every command.
- **TypeScript 6, not 7.** `typescript-eslint` caps its peer at `<6.1.0`.
