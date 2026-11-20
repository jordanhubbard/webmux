# Windows screen and renderer acceptance — 2026-11-19

The independent screen mirror (#355), independent underline/extended-state
model (#427), and aggregate ten-run VT budget correction (#458) meet their
scoped requirements. This record supersedes the pending screen/renderer gates
in the earlier [underline checkpoint](../xterm-underline-2026-11-17/README.md).
It does not qualify native coding-session recovery under #318 or a release.

The original qualified native source is
`0056000940ec0dbc96a010c3b88d349759c51c48`. At the audited main commit
`9c6a549b671a6a58d179614857ff5cc246b70b3f`, the entire screenstate, replayfilter,
nativeprovider, retained VT and ultraviolet trees, Go modules, npm lock, production
terminal component/adapters, replay test and independent reference qualifier
are byte-identical to that source. [receipt.json](receipt.json) records their
Git objects, original artifact digests, executable hashes and measured results.
This is source-scoped readmission of original evidence, not a fresh Windows run
of the entire current application.

## Native capture and independent renderer

Both dedicated original uploads were verified directly from their retained ZIPs,
including every uploaded member's hash, safe paths and absence of symlinks or
duplicate names. NAS sidecar files are not uploaded evidence.

| Original upload | Native capture executable | ConPTY cases | Three-stage replay reports |
| --- | --- | ---: | ---: |
| [AMD64, artifact14539750988](https://github.com/jordanhubbard/webmux/actions/runs/44541506649) | PE AMD64, independently hashed | 28 | 1,381 |
| [ARM64, artifact14540981112](https://github.com/jordanhubbard/webmux/actions/runs/44537924499) | PE ARM64, matching architecture receipt | 28 | 1,381 |

Each dedicated collector exits zero with 28 passing subtests, its passing parent,
and no failed/skipped tests. Its capture inventory equals the copy consumed by
the browser. Every replay report matches the original renderer at initial
attachment, continued output and recapture, with no replay-generated query replies.
Focus-out notifications are recorded separately from query replies.

These are actual jobs receiving input through their attached ConPTY streams:
`BEFORE`, `CHECK`, `AFTER` and `FINISH` each require the original live job's
response. All viewers must see the final response. The native test rejects
replayed private markers and a second process start. Six hidden-state cases
also probe legacy attachment and then verify the same owner instance/root PID.
Five refuse safely; `hidden-1049` is representable and accepts. Fresh v2 snapshot
tokens are checked at both attachment boundaries. Sequential tests may reuse
Windows PIDs; global PID uniqueness is not an acceptance condition.

Per upload, 259 comparisons use **bare xterm**, with unchanged parser,
hyperlink and reset methods. They include whole/byte-split hidden/contradictory
underline reference cases and additional restore-state controls. The other
1,122 comparisons use the production hyperlink-retention adapter on their
reference. Neither reference installs the snapshot adapter. In particular,
native ConPTY comparisons use that shared hyperlink reference and are not
described as bare-renderer controls. ARM64 Go/Node/capture are native; its browser
is AMD64 under emulation. There is no native ARM64 browser claim.

## Requirement coverage

| Requirement | Implementation and independently retained checks |
| --- | --- |
| Screen independent of output tail | Bounded VT mirror; current race control preserves a once-only header after 100,000 status rewrites, when absent from the last MiB; immutable captured image. |
| Atomic snapshot followed by live output | Owner mutex captures the image and registers the live queue before release; native three-attachment input/output barriers above, plus protocol ordering tests. |
| Visible and future terminal state | Text, styles, normal/alternate buffers, cursor, margins, wrapping, no-wrap edge cursors, modes, palette/default colors and links compared at all three boundaries. |
| Hidden underline/extended flags | Opt-in value-copy fields preserve independent flags, raw style/color and global links through SGR0/4/4:0/21/24/58/59, unrelated SGR, ESC/CSI/1048/1049/batched restores, REP, combining text, wide edits and resize. Bare reference cases plus native hidden-state cases remain distinct evidence. |
| Default behavior and provenance | Zero-value legacy behavior remains opt-in isolated; retained MIT licenses/upstream hashes and tests remain present. Current verifiers pass 241 VT originals/19 explicit patches and 104 ultraviolet originals/7 explicit patches. No second parser or unrelated flag-bit storage was introduced. |
| Query and private-request isolation | Filtered mirror input, discarded emulator responses, partial-control refusal, private marker exclusions and zero renderer query replies. Browser/clipboard/recovery requests cannot become replay or job input. |
| Bounded state | Geometry 1–500 columns by 1–200 rows; history at most 512 lines with a 16K-cell budget; 2MiB snapshot cap; parser/string/combining-flood controls. Unsafe display states refuse attachment. |
| Bounded viewer impact | Eight owner viewers, 64-frame live queues, five-second transport deadlines and one-second complete-boundary wait. A slow/incompatible viewer is detached while its existing terminal continues. |
| Versioned extension | Explicit `screen-v3` owner/v2 renderer capability; v1 semantics retained. Decoded v2 metadata is at most 192 bytes, columns at most 500, exact one-use token required. |
| Decoder refusal and cleanup | 256 controls, 79 packet splits, 107 cursor splits, 24 invalid payloads and four bad encodings; reused/disarmed/disposed tokens refused, REP cleared, no replies. |
| Link lifecycle and immutability | Scratch entries at most one at each boundary; active-link retention bounded to one; reset/direct reset/disposal release entries. Captured-state and future-image isolation retained. |
| Older owners/controllers/viewers | Negotiation only against advertised owner capabilities; explicit v1/v2 compatibility and unknown-profile refusal; owner/root identity preserved after native legacy probes. |

Ordinary ARM64 server tests separately pass controlled-terminal capability,
owner/refusal, snapshot-ordering, profile-transition and slow-viewer contracts.
They **skip** several native-provider/application lifecycle tests, including
`TestNativeProviderScreenReattachmentAndContinuedInput`,
`TestNativeProviderContinuationState`, and the general-suite invocation of
`TestNativeProviderSavedCursorCaptures`. Those skips are preserved in the receipt.
The dedicated native capture task runs the last test's 28 cases without skips;
it does not turn the other skipped lifecycle tests into passes.

A one-column wrapped-null state and other unsupported display features remain
explicit refusals. This acceptance requires safe refusal, not support for every
terminal protocol. A refusal neither starts nor kills the owned job.

## Ten-run aggregate budget

The original source `52a8527e7e8a3e0ed0d0af09eadbd9457eae9854` changes only
the new workflow step: retain ten full non-race repetitions in one process,
increase its aggregate timeout from four to ten minutes, preserve per-test JSON
with `Tee-Object`, propagate the Go exit code and upload the log with `always()`.
Reversing those workflow edits reconstructs the parent file byte-for-byte.
No production/browser deadline or test assertion changed in that correction.

| Original native evidence | Result |
| --- | --- |
| [Failed run44240510077](https://github.com/jordanhubbard/webmux/actions/runs/44240510077), job134586983884 | Original four-minute timeout retained as failure. |
| [Failed run44241259740](https://github.com/jordanhubbard/webmux/actions/runs/44241259740), job134589455925 | Original four-minute timeout retained as failure. |
| [Corrected source52a, run44242847564](https://github.com/jordanhubbard/webmux/actions/runs/44242847564), artifact14400652297 | 21,860 passing events: 2,186 identities exactly ten times; 234.947s. |
| [Final source005, run44541506649](https://github.com/jordanhubbard/webmux/actions/runs/44541506649), artifact14538863682 | 21,930 passing events: 2,193 identities exactly ten times; 291.4s. |

Every original passing identity remains at count ten; seven added reader-cleanup
identities also pass ten times. The same three explicitly skipped identities
remain skipped ten times. Both native artifacts have one package completion and
no failures. `TestGridMatchesUVBufferUnderRandomOperations` passes all ten runs.

The only subsequent retained-VT source changes are reader-test cleanup, diagnostic
formatting and provenance. Go AST ranges verify all 13 original conform/Kitty
test bodies remain byte-identical after normalizing only helper arguments and an
extra ignored return value. Reversing the grid's six diagnostic formatting
wrappers and helper reconstructs the original file exactly. Seeds, operations,
assertions and timing/allocation test bodies remain intact.

The final worker is Go1.27.4 windows/amd64 on AMD EPYC9V74, two cores/four logical
processors, XState `0x807`. This does not establish or repair #450's earlier
AMX/runtime corruption. The private Ice Lake ten-minute failure under #536
remains a failure; this budget correction is not its repair.

## Gates and reproduction

All ten original source005 workflows were read back as completed/successful,
including CI, packaging, ARM64 native screen and diagnostic/control workflows.
Their URLs and attempts are in the receipt. No hosted run was dispatched or
retried for this audit. The specific advisory issue #429 is closed; broader
security, provenance and release gates remain separate.

Current macOS component checks: 34 top-level race tests, 89 including subtests,
two packages, no failures/skips, plus both provenance verifiers. Whole-current-main
native qualification is not inferred from those checks or from source equality.

The private retained inputs are under
`/Volumes/ubumeh-scratch/webmux/puck/deck-414-zdzed69u/roadmap-0056000-final452/`
(three ZIPs, metadata and member manifests), its parent's
`52a-windows-vt-repeat.zip`, and `_build/windows-screen-current-audit/` (original
failure logs, initial artifact metadata, local logs and workflow readback).
Use the checked-in verifier and Go AST extractor against those inputs:

```bash
python3 docs/validation/windows-screen-acceptance-2026-11-19/verify-evidence.py \
  /path/to/audited/repo /path/to/roadmap-0056000-final452 \
  /path/to/52a-windows-vt-repeat.zip \
  docs/validation/windows-screen-acceptance-2026-11-19/extract-test-bodies.go \
  /path/to/go1.27.4 /path/to/windows-screen-current-audit
```

The verifier writes a new receipt only in the supplied audit directory. It checks
original uploads and current Git objects without starting native jobs. The two
failed original logs are hashed in the public receipt and retained privately in
full. Acceptance does not include #318/#438 coding lifecycle, #450 corruption,
#536 private timeout, distribution/signing/notarization, or release publication.
