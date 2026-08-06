# The web app

What the two pages show, the design system behind them, and the decisions that shaped both. `web/CLAUDE.md` holds the
must-knows for editing; this is the depth.

## The two pages

**`/`** says what the tool is in three sentences, puts the corpus totals beside them, and lists every session on the
machine. One `GET /api/sessions` brings all 725 in 273 KB, so sorting and filtering happen in the browser with no round
trip and no pagination. A row carries title, project, id prefix, start, elapsed time, subagent count, and transcript
size; the header row sorts on any of those. Under the list, a collapsed "What these numbers don't say" note holds the
four caveats a reader needs before trusting anything: thinking spans include model latency, lane time isn't elapsed
time, stall detection is a heuristic, and a thinking row's subject is borrowed from what came next.

**`/session/[id]`** analyses one session on load, in six stacked sections:

1. **The trace.** A stepped area strip under the header showing how many agents were producing at once across the span,
   with the peak called out. It's the shape of the session in one line: where the parallel waves were, and how much of
   the span had nobody working.
2. **The stat rail.** Elapsed, lane time, lanes, rows, each with its caveat printed under it rather than hidden in a
   tooltip. A number whose caveat lives elsewhere gets read wrong.
3. **Where lane time went.** A band bar (working / waiting / lost / compacting), then the donut with its legend as a
   table beside it. Hovering a legend row lights its slice.
4. **Who was alive, and when.** The swimlane, with a chip per workflow above it.
5. **What the agents reached for.** The tool donut, a strip of family shares over it, and the legend as a table beside
   it. Picking a slice or a legend row filters the sheet below to that tool's rows and scrolls to it.
6. **Every row.** The virtualized sheet.

## Decisions

### The API is reached through a Vite proxy, not by origin

`vite.config.ts` proxies `/api` to the backend port it reads out of the repo root's `.env`, the same file the Go server
reads. So the page is same-origin with its API: no preflight, no base URL in the app, and no failure when someone opens
`localhost:19428` instead of `127.0.0.1:19428`. The server's CORS allowlist (`docs/api.md`) still guards direct calls
from another page in the browser, which is the case it exists for.

### Aggregates first, rows second

The session page fetches `?rows=false` (364 KB on the largest session), renders the charts off it, and only then asks
for the rows (7.7 MB). Asking for both at once would have the server parsing the same transcript twice at the same time,
which delays the thing the reader is waiting for. The sheet section says how many rows it's fetching while it waits.

### Colour belongs to data

The chrome is ink on cool paper in light mode and ink on slate in dark, with one blue for links and focus. Every other
saturated pixel on the page stands for an activity kind, so a coloured button would read as a kind that isn't in the
legend. Work is cool (violet, blue, teal, green), the four waits are warm (orange, gold, bronze, taupe), the two ways a
session loses time are red and magenta, and compaction is slate. That makes "how much of this was waiting" answerable
from across the room, which is the first question anyone asks.

Waits also carry a diagonal hatch in the pie and the two trouble kinds carry dots, which is a second channel for anyone
who can't lean on hue and a reminder that those slices aren't work.

Kind colours live in `web/src/app.css` and are read back out by `theme.svelte.ts` for canvas, because ECharts needs
literals. Light and dark are two blocks in one file, switched by `prefers-color-scheme` with no class toggling, no
stored preference, and no flash on load.

### There are exactly two colour vocabularies, and each has its own legend

The tool breakdown can't use the kind colours: it's counting calls by what they were, not stretches of time by what an
agent was doing. So it has a second set, in the same file, and the page never puts the two in one chart. Each sits in
its own section behind its own legend, and the chrome between them stays neutral, so neither reads as the other.

The engine reports 15 tool classes, which is more than any palette keeps apart, so `classes.ts` maps them onto **seven
families** and the chart colours by family. That isn't a rollup for its own sake: because the pie draws families in a
fixed order, each family is one contiguous arc and the chart says "37% of this session's calls were file work" before a
reader touches the legend. Groups inside a family share its colour and are told apart by the 2px surface ring between
them, by the table beside the chart, and by the highlight the two share.

The order is the colourblind-safety mechanism rather than a mood: it decides which pairs ever touch. The seven slots
were validated with the `dataviz` skill's checker in both modes (2026-08-06), worst adjacent pair ΔE 8.7 light and 11.0
dark under protanopia and deuteranopia, 17.4 and 19.2 under normal vision, every slot over 3:1 on its surface. Re-run it
before changing a slot, and keep the order. The seventh slot is the neutral everything-else fold, so it sits below the
chroma floor on purpose.

### Type is the system font, deliberately

No web fonts. The tool reads transcripts off this machine and runs on it, and the terminal these sessions ran in is its
own vernacular, so the display face is the system UI face and the data face is the system monospace (SF Mono on a Mac).
What carries the personality is the treatment: tabular monospace numerals everywhere a number appears, uppercase
letterspaced micro-labels for structure, and durations set as the page's structural typography. It also means the page
looks right offline, which a local tool should.

### The entrance animation moves and never fades

Sections slide 6px into place, staggered, so the page assembles rather than blinking. They don't fade in, because a
browser composites text at partial opacity against the canvas behind it: faint ink at 40% alpha measures around 2:1
against paper, so a fade puts every word on the page under AA for as long as it runs, and someone reading during that
first half-second is the person the contrast ratio is for. Transform costs nothing in contrast and reads the same. It's
off entirely under `prefers-reduced-motion`.

### A thousand lanes

The largest session on this machine has 983 lanes, 977 of them spawned by 12 workflows and every one of those named
`workflow-subagent`. Three things together make it readable:

- **Workflows collapse.** One row per workflow, its bar the union of its members'. 983 lanes become 18 rows. A hole in a
  workflow row means every lane in it was quiet, which is worth seeing on its own.
- **Rows scroll rather than shrink.** The chart draws a fixed window of full-height rows (`MAX_VISIBLE_ROWS`) and moves
  a y-axis slider over the rest. A canvas 983 rows tall is not a thing a browser should be asked for.
- **An opened workflow is capped** at 150 lanes and says how many it held back, and its lanes are labelled by id prefix
  because their names are all the same.

### Sub-pixel segments are left alone

The swimlane's bars come out of `busySegments`, and the counts are small enough not to need coalescing: 100 segments
across the reference session's 31 lanes, 1,014 across the 983-lane one, and 310 in the largest workflow row after the
union (measured 2026-08-06). If a future session pushes that into the tens of thousands, coalescing segments below a
pixel is the move, but it would cost fidelity under zoom and nothing needs it yet.

### The sheet is virtualized, and its filters are split

TanStack Table owns sorting and the text search; four exact-match dropdowns (activity, tool, class, lane) narrow the
rows before the table sees them, because a dropdown doesn't need a filter row model. The table is rebuilt when its state
changes rather than mutated in place, which costs one pass over the rows and buys a component with no lifecycle
subtleties in it. The search box is debounced by 180 ms so a keystroke doesn't rebuild 22,000 rows.

The tool dropdown is bound to the page rather than owned by the sheet, so clicking a slice two sections up and picking
one from the dropdown are the same act. Picking a group shows both of its rows per call, the composing one and the
running one, because they're what the derivation produced and the sheet's job is to show the rows.

## Dependency notes

- **`@tanstack/table-core` 8, not 9.** The Svelte adapter only exists for v9, which was two days old when this was built
  and therefore inside the three-day release-age window. The core is framework-agnostic and wiring it to runes is a few
  lines, which is what shadcn-svelte does regardless. When v9 clears the window, `@tanstack/svelte-table` is the
  upgrade.
- **`@tanstack/virtual-core` rather than `@tanstack/svelte-virtual`.** The Svelte adapter wraps the core in stores;
  runes read the core directly in about fifteen lines.
- **`minimumReleaseAge` lives in `pnpm-workspace.yaml`**, not `.npmrc`. That's where pnpm 11 reads its settings, and npm
  rejects the key with a warning on every command.
- **TypeScript 6, not 7.** `typescript-eslint` caps its peer at `<6.1.0`.
