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
DECAWM disabled. Cursor motion restricts that coordinate before moving, while
HT preserves it, matching the captured xterm behavior. Default construction
keeps upstream behavior. **WebMux Screen does not enable this extension yet.**
The existing production refusal for ambiguous no-wrap snapshots remains.

`testdata/webmux-xterm-nowrap.json` contains 19 independently captured real
xterm results. The Go tests compare cursor, all rows and cell widths with both
whole writes and byte-split UTF-8/control sequences. These cases cover edge
addressing versus printing, repeated overwrite, wrap enable, movement, CR/LF,
tab, SGR, wide and combining text, saved cursor and alternate screen. They do
not establish snapshot/continuation, resize or real ConPTY acceptance.

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
