# Retained ultraviolet model extension

This directory retains every file from
`github.com/charmbracelet/ultraviolet@v0.0.0-20260812204455-68fa937c71be`
(commit `68fa937c71be6e737923b7c2793c117a94b6ef3d`) under its original MIT
license. `WEBMUX_UPSTREAM.json` records original and explicitly modified hashes.
The global module cache is unchanged.

WebMux adds an optional, immutable-by-value `UnderlineFlags` field to Style.
It preserves independent renderer underline/extended flag words instead of
conflating them with raw underline style/color and hyperlink data. Zero keeps
ordinary style-derived behavior. Cell copies, equality and zero checks retain
the flag value; ordinary visual rendering projects only visible style/link.
The source change is limited to cell styling and the two visual output paths.

This is a model extension for #427 under #355. It does not enable a production
snapshot path, remove refusals, or qualify native coding-session lifecycle.
Tests for ordinary ultraviolet behavior remain part of the required gates.
