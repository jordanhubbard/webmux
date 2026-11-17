# Independent underline-state model checkpoint — 2026-11-17

This is an incomplete implementation of #427 under #355/#318. Production Screen
does not enable `EnableXtermUnderline()` and still refuses hidden or
contradictory states. The existing v1 REP renderer profile is unchanged. Complete
snapshot painting, final active-pen restoration, versioned viewer negotiation
and actual native/application qualification remain required before merge.

## Model and provenance

The renderer saves foreground/background flag words independently of its live
extended underline style/color and hyperlink object. A single underline value
cannot represent those states. A small immutable value in `uv.Style` now carries
the optional flags through cells, saved pens, edits, reflow and snapshot wire
serialization. The zero value preserves ordinary style-derived behavior.

The complete MIT ultraviolet source is retained: 104 original files from
`v0.0.0-20260812204455-68fa937c71be`, with three explicitly patched originals.
Its original license, source hashes and full upstream tests are retained.
The vt module retains 241 originals with 15 explicit patches at this checkpoint.
Both provenance verifiers and nested-module race/vet gates run in CI; npm test
also includes both retained modules. Shared module-cache source was not edited.

The opt-in engine follows the locked renderer's SGR0/4/21/24/58/59 flag updates,
DECSC/DECRC and global extended state across normal/alternate screens. SGR58
changes raw color without arming the extended flag. SGR59 masks its sentinel
into an RGB-white value in xterm 6.0.0; the opt-in model preserves that quirk.
A stored cell without the extended flag has no extended object, while the pen
still retains its live object. Wide placeholders and repaired null cells carry
the current attributes. Unknown wire flag codes refuse before terminal mutation.

Ordinary ultraviolet output projects visual style/link. That projection does
not reconstruct complete hidden parser state and is not a snapshot encoder.

## Independent reference and local checks

The qualifier runs the unmodified locked xterm 6.0.0 bundle in real Chrome. It
reads only input names, bytes and resize steps from the fixture; it never reads
expected state or installs a snapshot adapter. It captures raw flag/style/color
state, visible underline, links, cursors and every cell before and after future
output. A link can force the renderer's visible underline to dashed while its
raw stored style remains different; both values are checked.

All 126 reference cases match the Go model, each with whole and byte-split
feeds at initial and continued boundaries. They cover ESC/CSI/1048/1049,
hidden/contradictory flags, indexed/RGB colors, unrelated SGR, REP/combining,
edits, wide-cell replacement/attributes and resize. A fresh renderer recapture
matches the checked-in corpus. Source/browser/corpus hashes are recorded in
`xterm-receipt.json`.

Additional checks passed:

- Full ultraviolet race suite, including copy/JSON/visible-output flag controls.
- Complete retained vt race suite: vt 120.613s, snapshot 1.168s, vtgen 1.144s.
  New tests cover snapshot wire round-trip, the initially restored cell,
  captured-image immutability, opt-in isolation and unknown-code refusal.
- Production Screen race suite: 5.635s; all 411 existing real Chrome replay and
  parser/lifecycle controls pass with the production model unchanged.
- Darwin root/nested-module vet, Windows AMD64 root vet, strict TypeScript
  source inventory, provenance and whitespace checks.

Reproduce the reference with:

```bash
node scripts/qualify-xterm-underline.mts \
  --chromium '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' \
  --output /path/to/new-reference.json
```

The Go model and internal wire tests are component evidence. They do not prove
complete ANSI/renderer-profile restoration, a second frontend attachment, actual
ConPTY support for these new states, or native coding-session lifecycle recovery.
Keep #427/#355/#318 open and this candidate draft until their remaining gates
are satisfied. No new release is qualified by this checkpoint.
