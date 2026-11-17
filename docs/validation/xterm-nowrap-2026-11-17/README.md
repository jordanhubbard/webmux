# No-wrap restoration qualification — 2026-11-17

[PR #422](https://github.com/jordanhubbard/webmux/pull/422) implements #421/#424
under the broader #355 screen-restoration requirement. It merged as
`7090e1e98629287020cdf569028297be63df8b8a` from tested PR head
`b12c7b5810fdc1ef94d0c3e133940ef6e21cf3fd`. The merge tree equals the qualified
PR tree. This checkpoint does not close the broader #355/#318 requirements.

## Source and gates

All 17 expected PR-head outcomes passed: 16 succeeded and Attach release bundles
was intentionally skipped. [CI 44113591855](https://github.com/jordanhubbard/webmux/actions/runs/44113591855)
includes the affected Windows race/protocol contracts, the complete retained
engine suite and real native/browser controls. The first USB job exhausted its
three bounded source-download attempts with HTTP429; an isolated retry passed
on unchanged source, with checksums, deadlines and acceptance assertions intact.
[Packaging 44113591900](https://github.com/jordanhubbard/webmux/actions/runs/44113591900)
also passed all required native bundle, package and artifact-verification gates.

Exact-merge [CI 44117724104](https://github.com/jordanhubbard/webmux/actions/runs/44117724104)
and [Packaging 44117724720](https://github.com/jordanhubbard/webmux/actions/runs/44117724720)
both passed on 7090e1e. These gates and artifact hashes are recorded in
`native-followup-results.json`.

The retained MIT vt 0.3.2 module contains 241 original files with per-file hashes.
Nine original source files have explicit patch hashes. Upstream default
behavior remains unchanged; CI and npm test include the full nested module
suite, and the provenance verifier checks original bytes, patches and additions.
The extension is opt-in at engine level and enabled by the Windows Screen mirror.
Reproduction is documented in
[`WEBMUX.md`](../../../webmux/server/internal/thirdparty/vt/WEBMUX.md).

## Behavior

The mirror distinguishes addressing the final column from printing past it
with DECAWM off. Whole/byte-split controls cover overwrite, wrap-mode changes,
wide/combining characters, saved and alternate-screen state, cursor movements,
tabs, edits, scrolling, margins, soft reset and resize through shrinking,
growing and regrowing. It preserves frozen cursor-line truncation, clipped
alternate-buffer columns, null cells versus printed spaces and xterm REP's
source-cell behavior.

Visible wide/narrow overlap is reconstructed with ANSI edge painting and DCH.
When ordinary printing would erase that overlap while reseeding REP, explicit
`Image.BytesForProfile("webmux-xterm-state-v1")` encoding adds a bounded final
OSC 7777 packet. Legacy `Image.Bytes()` still refuses that same unsafe image
before and after explicit encoding. Ordinary snapshots use ANSI and require no
profile metadata.

The locked xterm 6.0.0 adapter computes join/REP state through its own Unicode
service. A fresh 128-bit nonce arrives in transport metadata before snapshot
bytes. It authorizes one packet; malformed, oversized, invalid UTF-8,
control-containing, stale or reused packets cannot restore state. The adapter
has no host action or I/O operation and restores its property descriptor on
disposal. The reference terminal never installs this adapter.

Owners advertise `screen-v2`; controllers omit new request fields for old
strict v1 owners. The owner acknowledges any actual requirement and token
before output. Immutable metadata passes through the process and broker before
the reader starts. Every viewer join checks capabilities under the replay lock;
new-run and joining-viewer status configure the frontend before reset/replay.
Incompatible viewers receive a reload explanation while the same terminal job
continues. Controller/viewer detachment does not replace or stop that job.

## Evidence and limits

[Native artifact 14343035933](https://github.com/jordanhubbard/webmux/actions/runs/44113591855/artifacts/14343035933)
is bound to PR head b12c7b5. Inspection confirmed:

- 22 actual hosted Windows AMD64 ConPTY captures; each executed one native job.
  `TestNativeProviderSavedCursorCaptures` passed with zero skipped cases.
- 431 retained renderer receipts compare initial restoration, continuation and
  second attachment. All three phases match in every receipt; no terminal
  queries are replayed. Local focus reports are classified separately.
- Those receipts comprise 22 native captures and 409 portable Go snapshot cases.
  Two additional renderer lifecycle controls pass in the same browser job,
  for 433 browser tests total. The portable reference uses the unmodified
  renderer, apart from the independently qualified active-hyperlink profile.

The local renderer run passed 411 portable/lifecycle checks, including both
formerly failing whole/byte-split overlap-grow cases. Separate controls cover
all packet splits, bounds, stale/one-use tokens, combining/REP continuation,
normal parser resets and disposal. Three actual app browser workflows passed:
two-viewer query suppression, truncated replay, and controlled owned metadata
through two page attachments with stale-token rejection and no generated input.
The controlled-frame app test and controlled-terminal Windows negotiation tests
isolate transport/renderer contracts; they are not additional ConPTY captures.

Affected screenstate/session/httpapi/terminal race suites, Windows AMD64 vet,
strict TypeScript/source inventory, 372 existing frontend tests, lint, build,
provenance and whitespace checks passed locally. The last complete upstream
engine race run passed (vt 115.815s, snapshot 1.110s, vtgen 1.146s); hosted gates
also execute the complete retained module on each platform.

Hidden or contradictory extended underline flags still refuse attachment.
Other unqualified OSC display features, installed-service coding-session
recovery, broader CLI/policy coverage and physical ARM64 qualification remain
separate under #355/#318. This evidence does not qualify those cases or a new
release.

## Historical checkpoints

`native-followup-results.json` retains earlier failed and component results:

- Head 2c3f923: 22 native captures executed; 21 matched. Soft reset incorrectly
  retained focus-report mode. Correcting focus/paste/sync resets fixed it.
- Head 5a80f2c: all 22 fresh native captures matched, but two required portable
  overlap-grow controls still failed safely with `ErrUnsupported`.
- Head 15e9564: new client negotiation tests failed before their handshake
  because Go's long test-name temporary path exceeded Windows AF_UNIX bounds.
  A short owned temporary directory fixed the fixture without skipping tests
  or relaxing assertions. Head b12c7b5 qualified that correction.

Earlier 348/410-case browser runs, retained-input recomputations and their
limits are historical evidence, not substitutes for the fresh qualified head.
