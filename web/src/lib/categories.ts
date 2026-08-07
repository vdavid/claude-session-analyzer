/**
 * What colour a tool category is drawn in.
 *
 * **The taxonomy itself isn't here.** Which classes fold into which category, what each one is
 * called, and the order they're shown in all come from the engine and arrive on the API's
 * `toolCategories` (`docs/api.md`). This file holds only the design half: which CSS custom property
 * paints each one. A category name is data; a hex value isn't.
 *
 * So the names below are keys into a palette, not a definition of the buckets. The engine can add a
 * category without this file being wrong: an unknown name falls to the neutral slot rather than
 * throwing, which is the right failure for a page.
 *
 * The seven values in `app.css` were validated for colourblind-safe adjacency **in the order the API
 * serves them**, treated as a closed ring because a pie's last slice touches its first. Reordering
 * `Categories` in `internal/timeline` therefore changes which colours touch: re-run the validator
 * when you do (`docs/frontend.md` has the two commands).
 */

/** The slot everything unnamed lands in. It's the neutral, so it reads as "nothing to say here". */
export const NEUTRAL_CATEGORY_VAR = '--csa-tool-other'

/** Category name to the custom property that paints it, in both themes. */
const CATEGORY_VARS: Record<string, string> = {
    management: '--csa-tool-management',
    read: '--csa-tool-read',
    write: '--csa-tool-write',
    build: '--csa-tool-build',
    checks: '--csa-tool-checks',
    qa: '--csa-tool-qa',
    other: NEUTRAL_CATEGORY_VAR,
}

/** Every name this palette paints, for reading the stylesheet back out. Not the legend's order. */
export const PAINTED_CATEGORIES: string[] = Object.keys(CATEGORY_VARS)

/** The custom property a category is painted with. One with no slot of its own gets the neutral. */
export function categoryVar(category: string | undefined): string {
    return (category && CATEGORY_VARS[category]) || NEUTRAL_CATEGORY_VAR
}
