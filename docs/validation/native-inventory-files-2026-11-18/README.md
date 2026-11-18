# Refuse special files during native inventory

[#466](https://github.com/jordanhubbard/webmux/issues/466) fixes blocking opens
in the collector introduced by #462. On parent `5d20785`, an owned FIFO with no
writer blocks before the hash deadline can be checked. The mapped ELF-prefix
and copyright-document reads have the same failure. A regular path can also be
replaced with a FIFO between a path check and the open.

The collector admits regular files before opening them and validates the opened
descriptor before reading. POSIX nonblocking open closes the FIFO replacement
race; Windows byte mode preserves executable hashes. Tool, mapped ELF-prefix,
hash and copyright reads share this admission. Existing inode/change checks,
size/time bounds, package attribution and exclusive output remain in place.
Special files remain explicit unavailable observations, with no version probe.

The final two regression test methods exercise three real no-writer FIFO paths
and a replacement just before open, each in a child with a three-second parent
deadline. On the unchanged parent the three FIFO cases exceed their deadlines
and the replacement control fails. On the corrected source all eleven tests
pass, including all nine original test bodies unchanged. A real macOS ARM64
tool inventory, Python compilation and whitespace checks pass.

[Source and log bindings](source-manifest.json) record these local checks.
The existing Linux guacd live control and hosted exact-head qualification remain
required; prior native receipts are not substituted for this changed collector.
This is file-admission reliability, not vulnerability or license acceptance,
authenticated recovery or release completion. #91/#429 remain open.
