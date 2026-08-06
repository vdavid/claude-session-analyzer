/**
 * The stylesheet, read back out for the charts.
 *
 * The page has no theme switch: light and dark follow `prefers-color-scheme`, so CSS handles the
 * whole thing on its own. Canvas doesn't, though. ECharts needs literal colours, so this watches
 * the same media query the stylesheet does and re-reads the custom properties whenever it flips.
 * Charts subscribe to `theme.palette` and rebuild.
 *
 * `prefers-reduced-motion` rides along here for the same reason: chart animation is a JS option,
 * not a CSS one.
 */

import { browser } from '$app/environment'
import { KIND_ORDER, kindStyle } from './kinds'

const CHROME = {
    ink: '--csa-ink',
    inkMuted: '--csa-ink-muted',
    inkFaint: '--csa-ink-faint',
    border: '--csa-border',
    borderStrong: '--csa-border-strong',
    surface: '--csa-surface',
    sunken: '--csa-sunken',
    canvas: '--csa-canvas',
    accent: '--csa-accent',
    work: '--csa-band-work',
    wait: '--csa-band-wait',
    trouble: '--csa-band-trouble',
} as const

export interface Palette {
    dark: boolean
    kind: Record<string, string>
    chrome: Record<keyof typeof CHROME, string>
}

/** Stands in until the browser can be asked. Only ever seen if a chart renders before paint. */
const FALLBACK: Palette = {
    dark: false,
    kind: Object.fromEntries(KIND_ORDER.map((k) => [k, '#888888'])),
    chrome: Object.fromEntries(Object.keys(CHROME).map((k) => [k, '#888888'])) as Palette['chrome'],
}

function read(): Palette {
    const styles = getComputedStyle(document.documentElement)
    const value = (name: string) => styles.getPropertyValue(name).trim() || '#888888'
    return {
        dark: matchMedia('(prefers-color-scheme: dark)').matches,
        kind: Object.fromEntries(KIND_ORDER.map((k) => [k, value(kindStyle(k).cssVar)])),
        chrome: Object.fromEntries(Object.entries(CHROME).map(([k, v]) => [k, value(v)])) as Palette['chrome'],
    }
}

function createTheme() {
    let palette = $state(FALLBACK)
    let reducedMotion = $state(false)

    if (browser) {
        const scheme = matchMedia('(prefers-color-scheme: dark)')
        const motion = matchMedia('(prefers-reduced-motion: reduce)')
        palette = read()
        reducedMotion = motion.matches
        scheme.addEventListener('change', () => {
            palette = read()
        })
        motion.addEventListener('change', () => {
            reducedMotion = motion.matches
        })
    }

    return {
        get palette() {
            return palette
        },
        get reducedMotion() {
            return reducedMotion
        },
    }
}

export const theme = createTheme()
