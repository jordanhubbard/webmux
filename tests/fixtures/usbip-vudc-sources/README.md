# Pinned virtual USB test sources

The commit-named directory contains the eight unmodified Linux files already
pinned by `tests/usb-vudc-sources.json`. All60152 source bytes match that unchanged
manifest. Original SPDX GPL-2.0+ and copyright headers remain intact; GPL-2.0 is
the upstream license text from the same commit. `provenance.json` records exact
source/license URLs, lengths and hashes. Existing fixture patches apply only
after source verification in a disposable build directory.

These sources are exclusively for the virtual USB kernel qualification fixture;
they are not linked into or packaged with the WebMux application. This does not
clear deployment-specific component/license review under #91.

Acquisition prefers a hash-verified private cache, then this read-only bundle.
Every byte is still checked against the original manifest before cache/build
writes. Corrupt, incomplete, linked or nonregular bundle entries fail rather
than silently fetching replacements. Unbundled commits retain the existing
bounded network acquisition, retries and failure receipts. Receipt fields
`from_cache` and `from_bundle` identify the selected source.

This removes raw GitHub availability from a cold checkout of the pinned fixture.
It does not substitute source acquisition for real guest-kernel qualification.
Refs #550, #445, #120.
