#!/usr/bin/env node
/**
 * The one command that says whether the repo is green.
 *
 * Gates run in order, cheapest first, and a failure prints that gate's output in full and stops:
 * a wall of downstream noise from one broken file helps nobody. Scope it with an argument, either
 * a side (`go`, `web`) or a gate name (`vet`, `eslint`, …), so a docs change doesn't sit through a
 * Go test run.
 */

import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')

const gates = [
    { name: 'gofmt', side: 'go', run: ['gofmt', '-l', '.'], failOnOutput: true },
    { name: 'vet', side: 'go', run: ['go', 'vet', './...'] },
    { name: 'gotest', side: 'go', run: ['go', 'test', './...'] },
    { name: 'prettier', side: 'web', run: ['pnpm', '-C', 'web', 'exec', 'prettier', '--check', '.'] },
    { name: 'eslint', side: 'web', run: ['pnpm', '-C', 'web', 'exec', 'eslint', '.'] },
    { name: 'svelte-check', side: 'web', run: ['pnpm', '-C', 'web', 'run', 'check'] },
    { name: 'vitest', side: 'web', run: ['pnpm', '-C', 'web', 'exec', 'vitest', 'run'] },
]

const wanted = process.argv.slice(2)
const selected = wanted.length
    ? gates.filter((g) => wanted.includes(g.name) || wanted.includes(g.side))
    : gates

if (!selected.length) {
    console.error(`Nothing matches ${wanted.join(', ')}. Gates: ${gates.map((g) => g.name).join(', ')}, or go / web.`)
    process.exit(2)
}

let failed = false
for (const gate of selected) {
    const started = Date.now()
    const [command, ...args] = gate.run
    const result = spawnSync(command, args, { cwd: root, encoding: 'utf8' })
    const output = `${result.stdout ?? ''}${result.stderr ?? ''}`.trim()
    // `gofmt` reports unformatted files on stdout and still exits 0, so it's judged on its output.
    const broke = result.status !== 0 || (gate.failOnOutput && output.length > 0)

    if (broke) {
        console.log(`✗ ${gate.name}\n`)
        console.log(output || `${command} exited ${result.status} with nothing to say.`)
        failed = true
        break
    }
    console.log(`✓ ${gate.name} (${((Date.now() - started) / 1000).toFixed(1)}s)`)
}

process.exit(failed ? 1 : 0)
