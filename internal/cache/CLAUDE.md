# cache/: answers that survive between runs

Holds a summed answer per session on disk, so a question about the whole corpus doesn't mean parsing 3.8 GB again. Two
tiers: `Digest` is a few KB and answers a corpus query, `Detail` is tens of KB and adds the lane dimension. Rows are
never cached. The reasoning and the on-disk layout: `docs/cache.md`.

## Must-knows

- **A fingerprint covers every file the session is written across, never the lead alone.** A subagent appends to its own
  transcript and the lead's mtime doesn't move, so keying on the lead reads a changed session as unchanged. Size goes
  into the hash beside the mtime because one stat returns both, and it catches a coarse filesystem timestamp and a
  restore that puts an old mtime on new bytes.
- **Stat, parse, stat again.** `Corpus` stores a digest only when the fingerprint held across the parse. A live
  transcript grows while it's being read, and pinning a half-read parse to a key that stays valid is the one way this
  cache goes permanently wrong: every later run serves the wrong answer and nothing notices. The digest is still
  returned, it's the storing that's skipped.
- **Anything doubtful is a miss, not an error.** Absent, unreadable, half written, a `Version` from another derivation,
  a different fingerprint, or a zone whose day boundaries fall elsewhere: all misses. Re-deriving one session is about a
  second, and a stale digest is invisibly wrong.
- **Bump `Version` when the derivation's output changes.** That's how a rule change in `internal/timeline` invalidates
  every digest on disk, and nothing else does it.
- **The version guard hashes two fingerprints, and the failure says which moved.**
  `TestTheDigestVersionMovesWithTheDerivation` hashes the golden CSV (the derivation's output for one fixture session)
  and `timeline.ClassificationFingerprint` (the class and category mapping, whatever the fixture holds). The second half
  exists because the golden alone missed a rule change twice: version 4 split `lint` out of `build`, and the fixture's
  only compiler call is a `cargo build` that stayed a build, so every cached `cargo check` cell went stale with the
  golden sitting still.
- **Bump it for the one case neither fingerprint can see, too**: a stored number or field changing meaning while the
  rules stay put. Version 3 added `Totals.NetSeconds` and took the stalls out of a tool group's `Seconds`, off rows
  version 2 derived identically; version 5 added `category` to every stored tool group. A digest is an answer, and an
  answer under the old definitions is invisibly wrong even when the rows behind it never moved.
- **One file per session, never one per project.** Several agents query the corpus at once, and a file holding a
  project's sessions would need read, modify, write, which means a lock. Each file goes down as a temp file and an
  `os.Rename` in the same directory, so a reader sees one whole version or the other and the last writer wins with a
  digest that's just as valid.
- **Nothing is written inside `~/.claude/projects`.** Those transcripts are Claude Code's own irreplaceable data. `Open`
  refuses a cache directory that would land inside the transcript root, which is what XDG_CACHE_HOME pointing there
  would do.
- **Tier two counts as a hit only when it was asked for**, so a digest cached before anyone wanted the lane breakdown
  doesn't answer a query that needs one.
- **One unreadable session doesn't sink a walk.** `Corpus` skips it and reports it in `Failed` and `Errors`, because
  giving up on the first broken transcript costs a person the other 724 sessions.
- **`afterParse` in `warm.go` is a test seam, nil in every real run.** It's how `warm_test.go` moves a transcript
  between the parse and the second fingerprint, which can't be produced reliably any other way.

## Module map

- `digest.go`: `Version`, the two tiers, `Cell`, and `Build`, which sums a derived timeline into both. The payload
  contract.
- `fingerprint.go`: `Fingerprint`, over every file of a located session.
- `store.go`: `Open` and `OpenAt`, `LoadDigest`, `LoadDetail`, `Save`, `Clear`, `Info`, and `Prune`, plus the layout and
  the atomic write.
- `warm.go`: `Corpus`, the parallel walk over every session under a root, largest first.

Fixtures are two hand-written sessions under `testdata/projects`, one with a subagent lane and one without. Tests that
change a transcript copy the tree into a temp directory and pin every mtime first, so a fingerprint moves only when the
test moves it.
