# USB detach discovery resolution race

Issue [#514](https://github.com/jordanhubbard/webmux/issues/514).
CI `44316026958`, native Virtual USB job `134842232895`, fails on
`dea22c342c2ed0e8118f758cfaa58e464ebdb33e` during direct-cycle 5 detach.
The first four cycles pass; `/sys/class/block/sda` disappears after enumeration
but before `Path.resolve()` finishes. Its ENOENT escapes the existing detach
poll, which then aborts the otherwise unchanged native qualification.

Independently inspected artifact `14432038021` (`virtual-usb-gadget-build`),
SHA-256 `a02819031d4fa10bb5c2d68ea841bab9a4eb01872e85949fe0b0ec01d52e64dd`.
The original failure remains retained. This does not establish a USB kernel,
browser-shutdown, or relay stdout regression.

Skip only the disappeared candidate when resolution raises ENOENT or ENODEV,
then continue checking other entries. Existing exact fixture serial/controller/
whole-disk and readable payload checks remain. Permission, I/O, symlink-loop,
and other errors still propagate, as do discovery-root errors. The existing
poll, all 20 direct cycles, CLI/SSH controls and deadlines are unchanged.

All 13 sysfs discovery/readiness controls and nine qualifier-admission/source
cache controls pass locally. New deterministic cases cover disappearance during
resolution, a surviving candidate after the vanished one, and unexpected error
refusal. Against the parent helper, all three injected resolution-disappearance
cases fail; the remaining controls retain their results.

Fresh native kernel qualification is still required. Unit controls or a later
pass alone must not erase the retained failure or waive any direct/CLI/SSH gate.
This test-only change is stacked on #512 and does not alter audit/release gates.
