# Independent underline-state model checkpoint — 2026-11-17

> Historical checkpoint: the pending screen/renderer and dependency-advisory
> statements below describe this earlier source. The
> [2026-11-19 acceptance record](../windows-screen-acceptance-2026-11-19/README.md)
> supersedes those gates with original qualified native uploads, current source
> equality and explicit lifecycle limits. Native coding-session recovery remains
> separate under #318.

This is an incomplete implementation of #427 under #355/#318. The candidate
now selects the opt-in model for fresh Windows native owners (`screen-v3`),
and the frontend advertises v2 after validating its renderer layout. Existing
owners keep their running binary and job. Local bare-reference and application
contract checks pass. The first fresh Windows matrix found a combining-cell
recapture mismatch, described below. Corrected source `d76be88` now passes the
fresh native matrix; the dependency audit still blocks merge and release. #429 separately tracks the
dependency-audit release blocker. `Screen.New` retains the legacy model for
explicit legacy controls; the new owner calls `NewWithProfile`.

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

## Initial decoder checkpoint

`v2-decoder-results.json` retains the 37ca51e primitive checkpoint and its
historical source hashes: 256 component controls, 79 packet splits and the
original two-field/160-byte payload. The encoder below adds bounded columns
and a 192-byte limit. `encoder-results.json` records the subsequent a4f2920
encoder checkpoint. Current capability and bare-reference results are separate.

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
negotiation, native-owner-to-application second attachment, fresh ConPTY hidden-state
capture or the remaining #318 native coding-session lifecycle requirements.
Keep this PR draft and #427/#355/#318 open. Production and release remain gated.

## Bare reference and capability candidate

The 126 model-reference inputs now also have full Go snapshot comparisons
against a bare xterm instance, with whole and byte-split mirror feeds: 252
cases at initial, continued and recaptured boundaries. No reference adapter is
installed. Receipts check that the reference parser, hyperlink method and reset
method remain unchanged. The original shared-hyperlink baseline remains a
separate product-profile comparison. Exact Go-generated fixture bytes are now
retained per Playwright worker, alongside the renderer receipts.

The capability relation accepts v2 viewers for v1 snapshots and refuses
unknown profile names. New controllers request v2 from `screen-v3` owners,
request v1 from older `screen-v2` owners, and omit the renderer field entirely
for strict `screen-v1` owners. A reply cannot change the owner's advertised
replay version or introduce an unsupported requirement/token. Controllers that
predate `screen-v3` continue to refuse that unknown owner version; no job stop
or replacement is authorized by this refusal. Broker transitions detach only
incompatible viewers. New frontend instances validate v2 before advertising it
and arm the exact v1/v2 version named by trusted metadata.

The real application/Go-server contract tests cover both metadata versions,
two viewer attachments, stale tokens and no generated input. Handshake/output
frames are injected to isolate application ordering; these are not native-owner
or ConPTY acceptance. The pinned DOM renderer uses raw style0 for the CSS class
of an FG-only underline flag; the v2 test checks that actual `underline-0` state
rather than replacing it with the public getter's fallback style1.

Ordinary ANSI painting now reapplies captured protection after SGR0 and clears
its protection cache explicitly on reset. A bare-reference legacy ANSI control
checks protected text, SGR reset and subsequent selective erase without any
renderer packet. This retains older-viewer support for that ordinary state.

The Windows candidate adds v3/v2/downlevel/unsolicited-version/token contracts,
a real-owner controlled-terminal refusal test, and six new ConPTY producers
alongside the existing22. Capture records include fresh v2 requirements/tokens,
owner identity, one-job counts and the observed ANSI-only attach result from the current client. ConPTY
may normalize hidden state, so the actual streams and results must be inspected
before claiming native hidden-state coverage. Windows vet is compilation
coverage; no local Windows execution is claimed. See `capability-results.json`
for the local evidence and outstanding hosted qualification.

## Native-discovered standalone combining cells

Hosted Windows source `60715bd` executed all 28 ConPTY producers with no native
skips and one job per case. The native capture passed; its renderer gate passed
1,371 controls and failed the second capture of `hidden-wide-edit`. Initial and
continued output already matched. The reference retained an isolated U+0301 in
a zero-width cell; the mirror had attached it to the preceding wide character.

The locked xterm parser clears Unicode join state after every CSI, including
SGR and REP. Without join state it stores the mark in its own cell and advances
one column. The corrected opt-in model follows that rule, treats REP after such
a mark as a no-op, and distinguishes a standalone zero-width cell from a wide
placeholder during overwrites. ANSI row painting positions the cursor before
printing that cell, clearing parser join state without changing its pen. The
upstream default grapheme policy is unchanged. Provenance now declares 16
patched VT originals; all 241 originals remain retained.

Three portable regressions cover wide-edit/REP, SGR and cursor-movement
boundaries in whole and byte-split feeds, with additional bare-xterm comparisons.
Rebuilding snapshots locally from all 28 retained Windows streams passes the
renderer comparisons, including the original failing stream. That is recorded
stream evidence, not a fresh Windows execution of the corrected binary.

The native capture also observed five ANSI-only refusals among the six new
producers; `hidden-1049` was still representable at the tested boundary and
accepted. All probes retained the same owner/root identity and one job. These
checks use the current client without a renderer offer, not an old controller
binary. See `native-followup-results.json` for exact source, artifact and local
qualification scopes. Keep draft #428 and #427/#355/#318 open. The independent
#429 audit failure still blocks merge and release.

The corrected runtime checkpoint `d76be88a3df63989a19cb9d048363f81e04fab06`
subsequently passed fresh hosted Windows qualification in
[job 134297748797](https://github.com/jordanhubbard/webmux/actions/runs/44156277182/job/134297748797).
All 28 ConPTY cases executed with zero skips and one job each; the previously
failing `hidden-wide-edit` recapture now matches. The browser gate passed 1,384
checks in 3.3 minutes: 1,381 snapshot comparisons, including 259 bare portable
comparisons, plus three lifecycle controls. Every initial, continued and
recaptured state matches, no queries replay, bare reference methods remain
unchanged, and scratch links remain bounded to one per image. Native references
use the established shared-hyperlink profile, without the snapshot adapter.

[Artifact 14361587981](https://github.com/jordanhubbard/webmux/actions/runs/44156277182/artifacts/14361587981)
retains raw captures, native test events and renderer receipts (SHA-256
`e5e1cdcf441634e87791c30474251ca9947aa98e2845f4346c633d897490cf0b`).
CI checked synthetic merge `2879e2657658e3cee18cdf2f2610024235f75777`;
its tree `5762e6b422e5007f1bd651a77aafac856b3af272` exactly equals the
qualified PR head tree. The complete checkpoint gate set is 15 successful jobs,
one expected release-attachment skip and the failing dependency audit.
Packaging passed. These scoped results qualify this native snapshot correction,
not the remaining #318 authenticated coding-session or installed-service
requirements. No merge or release is qualified while #429 remains unresolved.
