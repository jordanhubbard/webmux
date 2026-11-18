# Browser companion on small screens

[#454](https://github.com/jordanhubbard/webmux/issues/454) addresses a reproduced
part of [#110](https://github.com/jordanhubbard/webmux/issues/110). At the combined
candidate `10b365c`, the real application gives the URL input only 95.6875 pixels
at 320×568. Always-visible profile setup pushes the remote page below the
initial view. The initial browser test fails its retained 180-pixel address
assertion; the baseline log and screenshot are retained privately.

The companion now fills narrow screens. Its header fits three actions across,
controls have a 44-pixel minimum height, and the address input occupies three
columns of a separate row. A native **Browser settings** disclosure holds
profile setup. Control ownership and lease actions remain outside it. Larger
screens retain the resizable drawer and ordinary address row.

The real Chromium/application test covers 320×568 portrait, 568×320 landscape
and a reduced-height 390×400 view. It checks address width, settings discovery,
horizontal overflow, a reachable remote viewport, once-per-tap native page
side effects, and returning to the same terminal. Input coordinates use the
actual resized worker frame; the test does not replace the remote worker.

Source bindings and local results are recorded in [source-manifest.json](source-manifest.json).
Screenshots are inspected for portrait and landscape and retained with private
logs. Browser-emulated touch and reduced heights do not establish physical
mobile keyboard, mobile Safari, real assistive-technology, provider sign-in,
security-key forwarding or Mosh acceptance. Those broader #110 requirements
remain open. Hosted exact-head qualification and the unchanged #429 audit gate
remain required before merge or release.
