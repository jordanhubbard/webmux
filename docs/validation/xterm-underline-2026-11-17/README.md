# Independent underline-state model checkpoint — 2026-11-17

This is an incomplete implementation of #427 under #355/#318. Production
`Screen.New` retains the v1 model and its existing refusals. The opt-in
`NewWithProfile(..., XtermUnderlineRendererProfile)` now includes a candidate
snapshot encoder. Native owners and the application do not yet select or
advertise it. Versioned owner/controller/viewer compatibility and fresh actual
native/application acceptance remain required before merge. #429 separately
tracks the dependency-audit release blocker.

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

## Initial unadvertised decoder checkpoint (37ca51e)

The Go codec and frontend decoder now have a separate v2 packet format that
combines the two current pen flags and REP cluster in one token-bound update.
The decoded JSON has exactly `u` (flag code 1..4) and `r` (empty or at most 64
UTF-8 bytes of printable text). The payload is bounded to 160 decoded bytes.
An empty cluster explicitly clears repeat state. Validation completes before
flags change or the single-use token is consumed. The decoder alters only the
current pen's two flag bits and parser join state; it retains the extended
object, other attributes, saved cursor slots, protection and charset. It checks
the renderer's attribute bit layout on a detached clone when arming v2.

Metadata must explicitly arm v2: a default/v1 token cannot authorize a v2
packet, and a v2 token cannot authorize a v1 packet. The component and native
transport still advertise v1, and the production screen encoder never calls
the new codec. This draft primitive is not yet a deployed protocol contract.

Isolated real-renderer checks in `screen-replay.spec.ts` compare 256 initial
and continued states against an unmodified reference (eight saved/hidden/link/
alternate/charset/protection states, eight continuations and four REP clusters).
To test the missing final tail independently, the target starts from the same
input as the reference and the test deliberately flips only its two current
pen flags before the packet. These are decoder controls, not Go-generated
snapshot or actual application attach comparisons. All 79 packet byte splits,
16 invalid schemas/text payloads, four malformed encodings, empty REP, stale
and reused nonces, metadata versions, disarming and disposal also pass without
generating terminal input. See `v2-decoder-results.json` for scope and hashes.

At this initial checkpoint, integration still needed per-cell ANSI construction
and full saved-slot-safe painting. The encoder checkpoint below supersedes that
investigation-only boundary. In particular, a scratch DECSC used to construct cell flags must not
replace the job's true saved slot during a final pending-wrap repaint. The
complete encoder must resolve and qualify this ordering before its v2 payload
and capability contract are finalized. Full native/application acceptance and
older owner/controller/viewer compatibility remain required.

## Opt-in snapshot encoder

The candidate now reconstructs each independent cell pen with ordinary ANSI
and a scratch DECSC/DECRC while painting. It installs the job's real saved slots
only after their buffer is painted. The active pen then uses ordinary SGR and
one final nonce-bound v2 packet, without repainting cells or overwriting those
slots. The packet restores independent pen flags and REP state, plus optional
logical columns `x` (active), `n` (normal saved) and `a` (alternate saved).
Every column is an integer from zero through the renderer's current width;
the encoder's absolute bound is 500. The payload now has a 192-byte decoded
limit. Unknown fields or invalid coordinates refuse before any state changes.
The initial two-field prototype remains accepted; v1 semantics are unchanged.

These columns preserve a cursor just past the right edge, which scratch DECRC
would clamp. Wrapped-row painting consumes a pending wrap before a scratch
restore. A temporary wide print constructs a genuine trailing null cell on a
wrapped row, then the next row replaces that temporary glyph. This preserves
null cells separately from printed spaces, including the state REP observes.
A wrapped null row in a one-column mirror remains explicitly unsupported.

A scratch hyperlink is never printed. One random, image-local named link ID is
reused across rows, bounding scratch entries to one per image. Renderer checks
assert that bound at both snapshot boundaries. Protection is reapplied after
painting SGR0, which clears xterm's protection bit. The opt-in engine follows
that reset too; its default upstream profile is unchanged. In the new model,
the filter passes SGR59 through instead of rewriting it to SGR58 RGB-white:
the two controls produce the same color but recompute hidden flags differently.

The expanded real-Chrome suite compares Go-produced snapshots against original
output at initial, continued and second-capture boundaries. Its reference uses
the shared hyperlink-retention adapter but never the snapshot adapter. Expected
states are produced by the renderer; the fixture generator reads only corpus
input bytes and resize plans, not expected cell states. Raw flags, raw underline
style, canonical colors, null strings, links, protection, cursor, wraps, modes
and existing visible state are compared.

The candidate covers the 409 existing portable inputs through the v2 path,
126 underline-reference inputs with whole and byte-split feeds (252 cases),
and nine additional saved/protected/charset/link/wrap/resize-after-attachment
inputs with both feed forms (18 cases). The original 409 production-path
comparisons and three renderer lifecycle/decoder controls remain in the same
suite. Exact run results and current source hashes are in
`encoder-results.json`; `v2-decoder-results.json` retains the earlier primitive
checkpoint and its historical source hashes.

This is component qualification. It does not yet prove native owner capability
negotiation, actual application second attachment, fresh ConPTY hidden-state
capture or the remaining #318 native coding-session lifecycle requirements.
Keep this PR draft and #427/#355/#318 open. Production and release remain gated.
