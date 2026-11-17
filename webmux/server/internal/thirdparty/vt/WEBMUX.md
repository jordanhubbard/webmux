# WebMux opt-in xterm no-wrap extension (issue #421)

This directory retains the complete MIT-licensed `tuios.dev/vt` v0.3.2 module,
including upstream tests. `WEBMUX_UPSTREAM.json` records the upstream commit and
the original SHA-256 of every file, plus current hashes for explicitly patched
upstream files. Run `python3 scripts/verify-vt-provenance.py` from the repository
root to verify that inventory. Keep the upstream LICENSE.

The upstream module has private print, parser and cursor state without a public
extension hook. The local module replacement allows a small opt-in extension
without modifying the shared Go module cache. `EnableXtermNoWrap()` retains the
logical cursor at column `Width()` after printing to the final column with
DECAWM disabled. Cursor motion and character edits restrict that coordinate before operating;
tabs, scrolling and soft reset preserve it. DECSTR re-enables wrapping. This
matches the captured xterm.js behavior. Default construction
keeps upstream behavior. **This draft candidate enables the extension in Screen. It is not merged or qualified for release.**
A no-wrap print over the trailing cell of a wide character intentionally
retains xterm.js's overlapping lead/last-cell representation. The independent Screen replay controls cover initial restore, subsequent output,
and second attach for this state.
The candidate replaces the ambiguous no-wrap refusal with logical cursor replay;
separate unsupported underline state still refuses replay.

`testdata/webmux-xterm-nowrap.json` contains 138 independently captured real
xterm results. The Go tests compare cursor, all rows, individual cell contents and cell widths with both
whole writes and byte-split UTF-8/control sequences. These cases cover edge
addressing versus printing, repeated overwrite, wrap enable, movement, CR/LF,
tabs, SGR, erases, character and line edits, scrolling, soft reset, wide and
combining text, saved cursor and alternate screen. Each of the 46 base cases
also has subsequent output with wrapping disabled and re-enabled. A separate browser suite tests actual Screen snapshots and continuation.
Resize, margins and real ConPTY acceptance remain to complete.

After installing the repository's locked npm dependencies, reproduce the
reference using an installed browser (or omit `--chromium` for Playwright's):

```sh
node scripts/qualify-xterm-nowrap.mts --output /path/to/new/reference.json \
  --chromium /path/to/chromium
```

The capture reads only names and input bytes from the fixture, then records
results directly from the installed xterm. A receipt records browser version
and SHA-256 digests of xterm, qualifier, fixture and output. Review differences
before replacing expected results; do not regenerate them from the Go engine.

This is a nested Go module: root `go test ./...` does not include its tests.
Run `go test -race ./...` and `go vet ./...` inside this directory. CI does both
explicitly on each Go-server platform, in addition to the server tests.

Before enabling the extension, complete Screen snapshot restoration and
continuation comparisons (including second attach and resize), broaden cursor
control coverage, and run real Windows ConPTY cases. Issue #355 also retains
its separate hidden-underline and lifecycle acceptance requirements.

The profile also retains null cells separately from printed spaces: xterm's
REP reads literal source-cell text at the logical cursor minus the previous
character width. The snapshot painter uses erase-and-move for null cells.
The default upstream profile retains its original blank representation.
