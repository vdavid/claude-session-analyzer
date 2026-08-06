# components/: the display layer

Thin by design. Business logic belongs in the Go engine, shaping in `src/lib/transform/`, and these render what those
two hand over.

## Must-knows

- **`DataSheet.svelte` builds its TanStack table with a full state.** `table.getState()` returns `options.state`
  verbatim, so a partial one leaves column pinning undefined and the first header read throws. The default state only
  exists once the table is built, hence the `setOptions` right after `createTable`.
- **Row height is fixed**, which is what lets the virtualizer measure nothing. Long text is clipped to one line and
  carried in full on the row's `title`. Anything that can wrap breaks the row alignment.
- **Sort an instant through a numeric accessor.** RFC 3339 strings look sortable and aren't: the engine trims trailing
  zeros, so `…16.703Z` sorts after `…16.7Z` as text.
- **The scroll container carries `tabindex="0"`** with an a11y suppression: a scrollable region has to be reachable by
  keyboard, and 22,000 rows behind a pointer-only scroll is not usable.
- The search box is debounced because a keystroke rebuilds the row model over every row.

## Module map

- `DataSheet.svelte`: the virtualized sheet. Sorting and filtering through TanStack Table, windowing through TanStack
  Virtual, plus three exact-match dropdowns applied before the table sees the rows.
- `KindLegend.svelte`: the pie's legend as a table, with each kind's caveat. Hovering a row lights its slice.
- `BandBar.svelte`: the eleven kinds rolled into working, waiting, lost, and compacting.
- `StatRail.svelte`: numbers with their caveat under them rather than in a tooltip.
- `Notice.svelte`: the empty and unreachable states. Never says "error" or "failed".
- `charts/`: see its own `CLAUDE.md`.
