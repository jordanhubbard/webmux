# Opt-in no-wrap engine checkpoint — 2026-11-17

Tracks #421 under #355. This checkpoint preserves the engine distinction
between addressing the final column and printing past it while DECAWM is off.
The draft candidate now enables the extension in Screen and replaces its
no-wrap refusal with replay. It is not merged or qualified for release.
The current checkpoint below supersedes the historical results that follow.
Fresh qualification of the new renderer-profile transport remains required.


## Current renderer-profile checkpoint

Issue #424's overlap-grow state now has an explicit negotiated encoder:
`Image.BytesForProfile("webmux-xterm-state-v1")`. The legacy `Bytes()` API still
returns `ErrUnsupported` for that same image before and after profile encoding.
Ordinary snapshots remain standard ANSI and carry no profile requirement.

The encoder restores visible cells with ANSI, then appends a bounded OSC 7777
packet containing the last printed cluster and a fresh 128-bit snapshot nonce.
Only the version-qualified adapter for the locked xterm 6.0.0 renderer consumes
it. It restores parser join/REP state through xterm's own Unicode service and
leaves the independent reference renderer unmodified. A matching token comes
from transport metadata, never terminal output. Valid tokens are consumed once;
malformed, oversized, control-containing, invalid UTF-8 or stale-token packets
cannot restore state. The adapter has no host action or I/O operation and
restores its property descriptor on disposal.

Native owners now advertise `screen-v2`. The controller sends the profile field
only to a v2 owner, leaving old owners' strict frame decoding compatible. The
owner acknowledges any actual requirement and nonce before sending snapshot
bytes. Client metadata is immutable; the broker records it before starting the
reader. Every join checks capabilities under the replay lock. An incompatible
viewer is detached with a reload explanation while the same owned terminal
continues running. New-run and joining-viewer status configure the frontend
before its queued reset and replay. No process restart or navigation/input
replay is introduced.

The first complete local profile run passed all 411 real Chrome renderer checks
(initial, continued and second attachment comparisons plus lifecycle controls).
This includes the two formerly failing overlap-grow controls and byte-split
packet, malformed/stale token, one-shot, normal parser reset and disposal checks.
A controlled-frame test of the actual app passed through two page attachments:
it verified capability advertisement, metadata-before-output, REP, rejection
of stale tokens and no generated terminal input. That app test isolates the
frontend contract; it is not native ConPTY evidence.

Fresh Windows artifact 14337553952 at PR head 5a80f2c (workflow 44105321159)
contains 22 actual native ConPTY captures. All 22 executed with one job each and
passed initial, continued and second-attach renderer comparisons without replayed
queries. Its overall gate still failed the two static #424 controls. This proves
the prior native engine fixes, not the new profile/transport changes in this
checkpoint. Fresh Windows profile protocol contracts, actual ConPTY captures
and all hosted checks remain required before this draft can merge. Hidden
underline and wider native lifecycle qualification remain under #355/#318.

## Historical checkpoints

The retained MIT module has 241 original files with recorded upstream hashes.
Patched files are emulator.go, utf8.go, cc.go, csi_cursor.go, handlers.go,
esc.go, grid.go, screen.go and reflow.go; new extension,
reference tests and provenance files are listed by the integrity verifier.
Upstream default behavior remains covered by the full original suite.

Validation performed locally:

- Full retained module suite, then full race suite and vet passed. Race results:
  vt 114.670s, snapshot 1.158s, vtgen 1.136s (Go 1.27.3, Darwin ARM64).
- Existing server screenstate race suite passed (Go 1.27.4, Darwin ARM64).
- Strict TypeScript/source inventory checks passed.
- Real Chrome xterm recapture equals all 138 checked-in expected results.
  The receipt records browser version and hashes of qualifier, installed
  xterm bundle, fixture and output. Comparison decodes JSON, so formatting
  differences do not affect acceptance.
- Provenance verifier and staged whitespace checks passed. Terminal recordings
  and Unicode fixtures retain their significant original bytes.

The Go reference test runs each case as a whole write and one byte per write,
including split UTF-8 and combining characters. It compares cursor, all four
screen rows and second-row cell widths and individual cell contents on all four rows. It does not prove untested controls,
resize, margins, Screen snapshot replay, continuation, second attach or native
Windows ConPTY behavior. Complete those comparisons before enabling the
extension or removing production refusals. Hidden underline remains separate.

Reproduction instructions are in
`webmux/server/internal/thirdparty/vt/WEBMUX.md`.

The expanded 46 base cases add backward/forward tabs, character and line
edits, erases, scrolling, soft reset and cursor controls. Every base case also
runs with subsequent printing, with DECAWM unchanged and re-enabled (138 total).
This exposed fixes for edit-time cursor restriction, scroll/tab cursor retention,
DECSTR restoring DECAWM, and xterm's overlapping wide-lead/trailing-cell state.
The latter remains an explicit snapshot-encoding qualification boundary.

## Snapshot integration checkpoint

Actual Go Screen snapshots were independently rendered in Chrome/xterm. All
348 browser tests passed, covering the previous screen cases, 138 no-wrap
states each with whole/byte-split input (276 controls), and four additional
REP source-cell states each whole/byte-split. Every snapshot control compares
initial restoration, subsequent output and a second attachment, plus modes,
cell contents, widths, styles, links, wrap flags, palette and cursor, with no
replayed terminal queries.

The first no-wrap run passed 274/276; the two failures exposed REP reading an
untouched blank after a discarded wide character. Retaining null cells distinct
from printed spaces and painting those nulls with erase-and-move fixed that
case. Explicit controls include a printed space, ASCII source, wide placeholder
and a colored null. A follow-up full run exposed loss of an explicitly printed
space in the upstream grid's blank-row optimization; recording it also preserved
the previously tested alternate-screen wrapped blank. The final complete run
passed 348/348 in 1.1 minutes.

Nine real ConPTY controls have been added for edge addressing, advancement,
overwrite, wrap enable, wide fit/overlap/discard, backspace and soft reset. They
have not yet run on this candidate. Do not treat these test definitions as
native Windows evidence. Existing hidden-underline refusals remain.

The full upstream default-profile race suite exposed a missed grid reconstruction
in reflow after adding the configurable null cell. That reconstruction now
retains the configured erase cell. Generated extent, resize-conservation,
metamorphic and render-fuzz regression cases then passed together. The full
race suite and vet then passed after that correction; the intermediate failure
is not release evidence.

## Resize, margins and real ConPTY follow-up

The candidate adds whole/byte-split resize controls for addressed, advanced,
wide, overlapped, saved and alternate-screen states, through widening,
narrowing and growing again. Scroll-region controls cover bottom wrap/index,
top reverse index and saved-cursor clamping. Fixes preserve xterm's frozen
cursor line, clipped columns and saved coordinates, retain alternate-buffer
hidden columns, flush an already drawn grapheme before resize, and restrict
reverse-index before testing scroll margins.

The visible interior wide/narrow overlap can be reconstructed in ordinary
ANSI by building it at the edge and moving the pair left with DCH. A retained
parser repeat state still cannot be seeded by the current encoder without
clearing that lead. Issue #424 blocks #421. The candidate refuses that unsafe
state, and its whole/byte-split browser acceptance tests remain failing. They
are not skipped, and this PR must remain draft until they pass faithfully.

Actual Windows artifact 14337006971 from CI 44100418808/PR head 2c3f923 contains
22 executed ConPTY captures, each from one native job. The renderer comparison
passed 21/22; `nowrap-soft-reset` differed only in focus-report mode (1004):
xterm reset it while the mirror retained it. The candidate now also resets
focus reports, bracketed paste and synchronized output on DECSTR. Recomputing
snapshots from all 22 retained actual ConPTY input streams using the current
mirror passed all 22 independent renderer comparisons locally. That is a
retained-stream component check, not a fresh Windows execution of this candidate.
The fresh native gate remains required.

To recompute current mirror snapshots from retained original/future inputs,
set `WEBMUX_SCREEN_REFERENCE_INPUTS` to a JSON array of bounded objects with
name, cols, rows, original and future. The qualifier generates new snapshots;
recorded snapshots are not reused. Select `recorded-conpty` in the browser suite.

The final expanded local browser run executed 410 checks: 408 passed; the two
required whole/byte-split overlap-grow cases still report `ErrUnsupported`
and fail their support assertion. This is an incomplete qualification result,
not a green gate. Full retained-module race/vet passed again (vt 115.815s,
snapshot 1.110s, vtgen 1.146s); affected screenstate race and platform vet passed.
