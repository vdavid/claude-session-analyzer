# The cache

What `internal/cache` keeps on disk, why so little of it, and how it goes stale. Read before changing a stored shape or
the key.

## Why it exists

Corpus questions ("did my agents prefer codegraph or grep this year") need every session, and parsing 3.8 GB serially
takes minutes. Cache turns that into one cold run, then milliseconds. Single-session questions never needed it: one
parse is about a second.

Measured 2026-08-07 on this machine (735 sessions, 3.8 GB, 16 cores): cold `cache warm` 3.9 s, warm corpus query 0.24 s.

Nothing else is cached. The HTTP API still parses on request.

## Two tiers, loaded lazily

- **`Digest`**, 14 KB on average, 106 KB at worst. Session metadata, totals, and cube cells with lane dimension dropped.
  Every corpus query reads only this: 10.3 MB over 735 sessions.
- **`Detail`**, 32 KB on average, 1.3 MB at worst (the 983-lane session). Same cells with lane and agent kept, plus one
  entry per lane. 23.2 MB over the corpus, and loaded only when a query names `lane` or `agent`.

Both measured 2026-08-07.

Both hold `agg` cube cells at grain kind, class, group, leaf, tool, day. A query rolls them up further with the same
`agg.RollUp` that built them, so a cached answer and a fresh parse can't disagree.

**Rows are never cached.** They'd be 350 to 400 MB of JSON over the corpus (estimated from the reference session's 6.5
MB against its 67 MB transcript), they're useless to a corpus query, and re-deriving one session's rows is about a
second. A live session would rewrite megabytes on every query. If single-session latency ever gets annoying, tier 3 is
one more file behind the same key, no redesign.

## Where

`~/.cache/claude-session-analyzer/<hash of transcript root>/<project-slug>/<session-id>.digest.json` (and
`.detail.json`), honoring `XDG_CACHE_HOME`.

- **Never inside `~/.claude/projects`.** That's Claude Code's own irreplaceable data, owned by another program that may
  prune or restructure it. No bug in our prune logic should be able to reach it.
- Root is hashed into the path because `DefaultRoot` honors `CLAUDE_CONFIG_DIR` and `--root` exists. Two roots must not
  share a cache.
- **One file per session, never one per project.** Several agents query at once. Per-project files would need
  read-modify-write, so a writer refreshing 2 of 30 sessions could truncate the other 28. Per-session files make every
  write an independent temp-plus-rename: no lock, no merge, and a half-warm cache is only fewer files.

## The key

A digest is valid when all three match:

1. **Fingerprint.** Hash of (path, size, mtime) over every file the session is written across: lead transcript plus
   every lane file, across every slug when the session entered a worktree. Keying on the lead alone is silently stale,
   because a subagent appends to its own transcript and the lead's mtime doesn't move. Size is free (one `stat` returns
   both) and covers coarse mtime and mtime-preserving restores.
2. **`Version`.** The derivation the answers came from. See below.
3. **`Zone`.** Day buckets are cut at local midnight, so a digest built elsewhere answers "how much in July"
   differently.

Anything doubtful, including an unreadable or malformed file, is a miss, not an error. A stale digest is invisibly
wrong; a miss costs a parse.

## Invalidation when a rule changes

This is the one way the cache can be wrong with nothing downstream able to tell. Change a rule in `internal/timeline`
without bumping `Version`, and every digest on disk keeps being served under the old rules.

So it's enforced, not remembered: `TestTheDigestVersionMovesWithTheDerivation` hashes
`internal/timeline/testdata/golden/timeline.csv`, which already holds the whole derivation. Change what the derivation
outputs and that test fails with the fingerprint to write down, next to a `Version` to bump.

## Staleness during a write

Stat, parse, stat again. A digest is stored only when the fingerprint is unchanged across the parse. A transcript grows
while it's read, and pinning a half-read parse to a fingerprint that stays valid is permanent, invisible corruption. The
digest is still returned to the caller, just not written.

The live session re-parses on every query, by design: its fingerprint moves constantly.

## Warming

`claude-session-analyzer cache warm` parses everything missing. Parallel across `runtime.NumCPU()` workers, largest
session first so the 67 MB one doesn't finish alone after everything else. Progress on stderr. A session that fails to
parse is skipped and counted, never sinks the run.

`cache info` reports what's stored, `cache clear` removes it, and `--no-cache` on a query bypasses it entirely.

## Costs

Measured 2026-08-07 (735 sessions, 3.8 GB, 16 cores). `cache info` reports what's current.

- Cold `cache warm`, both tiers: 3.9 s wall clock, 43 s of CPU across 11 cores.
- Warm corpus query, tier one only: 0.24 s. Tier two, when a query names a lane or an agent: 0.28 s.
- On disk: 10.3 MB of digests, 23.2 MB of details, 33.4 MB total against 3.8 GB of transcripts.
- Invalidation sweep costs what `session.List` costs, about 150 ms, because it's the same directory scan.
