# Opt-in no-wrap engine checkpoint — 2026-11-17

Tracks #421 under #355. This checkpoint preserves the engine distinction
between addressing the final column and printing past it while DECAWM is off.
The extension remains disabled in production Screen; existing refusal guards
remain. This is not snapshot/continuation or ConPTY acceptance.

The retained MIT module has 241 original files with recorded upstream hashes.
Only emulator.go, utf8.go, cc.go, csi_cursor.go, handlers.go and esc.go are patched; new extension,
reference tests and provenance files are listed by the integrity verifier.
Upstream default behavior remains covered by the full original suite.

Validation performed locally:

- Full retained module suite, then full race suite and vet passed. Race results:
  vt 115.895s, snapshot 1.145s, vtgen 1.143s (Go 1.27.3, Darwin ARM64).
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
