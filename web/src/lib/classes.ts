/**
 * How a tool call's class is drawn and described.
 *
 * The engine reads a call's **class** off what it was doing: 15 of them, from `checker` to `other`
 * (`internal/timeline/tool.go`). Fifteen colours is more than any palette can keep apart, so the
 * chart colours a call by its **family** instead: seven, in a fixed order, each a validated slot in
 * `app.css`. The order is the colourblind-safety mechanism rather than a mood, so don't reorder it
 * without re-running the `dataviz` skill's validator over both modes.
 *
 * A family isn't a rollup for its own sake. Colouring by family is what makes the pie say "37% of
 * this session's calls were file work" at a glance, with the legend beside it naming the tools.
 *
 * Look a class up by name, never by position. A class the engine grew that this file doesn't know
 * falls into the neutral family rather than throwing.
 */

export type ToolFamily = 'teamwork' | 'files' | 'gates' | 'vcs' | 'search' | 'services' | 'shell'

export interface FamilyStyle {
    family: ToolFamily
    /** What the legend calls it, in sentence case. */
    label: string
    /** The CSS custom property holding its colour, in both themes. */
    cssVar: string
    /** What sits in it, for the legend's own caption. */
    description: string
}

/**
 * The fixed order. The pie draws families in it, so these are the only pairs that ever touch, and
 * they're the pairs the palette was validated on.
 */
export const FAMILY_ORDER: ToolFamily[] = ['teamwork', 'files', 'gates', 'vcs', 'search', 'services', 'shell']

const FAMILIES: Record<ToolFamily, FamilyStyle> = {
    teamwork: {
        family: 'teamwork',
        label: 'Teammates and people',
        cssVar: '--csa-tool-teamwork',
        description: 'Spawning a teammate, messaging one, or putting a question to a person.',
    },
    files: {
        family: 'files',
        label: 'Files',
        cssVar: '--csa-tool-files',
        description: 'Reading, writing, moving, and listing.',
    },
    gates: {
        family: 'gates',
        label: 'Builds, tests, and checks',
        cssVar: '--csa-tool-gates',
        description: "A compiler, a test runner, or the project's own gate. The calls that earn their minutes.",
    },
    vcs: {
        family: 'vcs',
        label: 'Version control',
        cssVar: '--csa-tool-vcs',
        description: 'Git and the GitHub CLI.',
    },
    search: {
        family: 'search',
        label: 'Search',
        cssVar: '--csa-tool-search',
        description: 'Grep, glob, find, and the search tools.',
    },
    services: {
        family: 'services',
        label: 'Services and the web',
        cssVar: '--csa-tool-services',
        description: 'An MCP server, or fetching and searching the web.',
    },
    shell: {
        family: 'shell',
        label: 'Everything else',
        cssVar: '--csa-tool-shell',
        description:
            'A shell command that is none of the above, a dev server, a call whose whole job was to block, and any tool this page has no family for.',
    },
}

/** Every class the engine can report, mapped to the family it's drawn in. */
const CLASS_FAMILIES: Record<string, ToolFamily> = {
    agent: 'teamwork',
    ask: 'teamwork',
    'file read': 'files',
    'file write': 'files',
    checker: 'gates',
    build: 'gates',
    test: 'gates',
    git: 'vcs',
    search: 'search',
    mcp: 'services',
    web: 'services',
    shell: 'shell',
    'dev server': 'shell',
    wait: 'shell',
    other: 'shell',
}

/** The family a class is drawn in. One the engine grew since falls into the neutral bucket. */
export function classFamily(className: string | undefined): ToolFamily {
    return (className && CLASS_FAMILIES[className]) || 'shell'
}

export function familyStyle(family: ToolFamily): FamilyStyle {
    return FAMILIES[family]
}

/** The family a class is drawn in, ready to render. */
export function classStyle(className: string | undefined): FamilyStyle {
    return FAMILIES[classFamily(className)]
}
