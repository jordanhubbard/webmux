# Opt-in no-wrap engine checkpoint — 2026-11-17

Tracks #421 under #355. This checkpoint preserves the engine distinction
between addressing the final column and printing past it while DECAWM is off.
The draft candidate now enables the extension in Screen and replaces its
no-wrap refusal with replay. It is not merged or qualified for release.
Native ConPTY, resize and margin acceptance remain incomplete.

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
