# Browser storage source and relocation acceptance

[Issue #763](https://github.com/jordanhubbard/webmux/issues/763) asked for private
browser storage outside the account home root, preservation of existing data,
retained shared-path behavior for packaged Chromium, and real-browser cleanup.
The implementation entered main through #766 and is present in v1.4.7. This audit
reconciles those scoped criteria against the original retained artifacts and the
current source. It adds documentation and sanitized evidence only.

| Criterion | Direct evidence |
| --- | --- |
| Private allocation and saved-profile boundaries | `paths.go`, `profile.go`, `worker.go` and `process_exit_linux.go` are byte-identical between native-qualified `169dc3da16e2dc7979de7145a3644046c563ba20`, current base `a1deb5a46b8ef32e2104a54c6b619578cac52e33`, and published v1.4.7. The receipt records each file hash. |
| Existing data preserved | The original command first required no open files under the seven selected roots. It compared regular-file SHA-256/size, modes and symlink targets before and after each rename, then wrote its successful receipt. All 4,208 entries matched. |
| Relocated data still present | All seven destinations remain under the private browser tree, its root/profile/tmp directories have mode 0700, and no `webmux-*` entries remain directly under the account home. |
| Real macOS browser and application behavior | The unmodified GUI `make test` at `2cc3a26d249f9b04e17e0a85f49466128ba47855` completed with exit 0: 500 frontend, 297 helper and 1,418 E2E passes, with 22 existing skips. The actual retained stdout/stderr hashes match its receipt. |
| Strict Snap shared storage and cleanup | The native Linux ARM64 archive at `169dc3da` matches its original checksum. It contains 50 top-level browser passes, zero failures and one fixture-child skip, plus all ten original UI cases. The primary browser and observed children retain enforced Snap confinement without forbidden primary flags. Owned home entries and retained VM inputs are unchanged; shutdown is normal. |

The original relocation receipt is retained privately at
`_build/make-audit-2026-11-26/home-relocation.json`; its SHA-256 is
`c1e9dc7451b93bb718ce7bb81e7cc1e361053270cc5a36fe8a9bf452943c432a`.
The successful original execution record was recovered and inspected, including
the actual manifest comparison and exit 0. This verifies the historical move;
it does not invent a new before-move snapshot or rerun the relocation. The
macOS successful receipt is `gui-interactive-make.json` under
`_build/release-targets-767/`. The earlier `gui-make-test.json` and
`gui-resize-make.json` still record exit 2 and are not counted as passing runs.

The Snap receipt is `snap-pidfd.json` in that same audit directory. Its actual
`guest-results.tar.gz` has SHA-256
`443417cecf8b2853fc4c123d4f9263e37651b077840d6a5621acc84d3bca32bf`.
This audit reads the successful `pidfd/` outcomes and logs separately from older
failed material retained in the archive. The earlier Snap attempts retain their
own failed outcomes. The original two Windows-only NetLog subcases remain skipped;
the ordinary fixture-child skip is exercised separately by its parent control.
The [original native audit](../make-target-audit-2026-11-26.md#native-linux-snap-storage-and-shutdown-763--771)
records the shutdown-ordering failure, pidfd correction, source/platform limits
and unchanged two-second cleanup bounds. No test assertion, deadline, browser
protection, host policy or source input was changed by this reconciliation.

[receipt.json](receipt.json) records the sanitized hashes, counts, native source
bindings, failed attempts and current source comparison. Browser contents,
profiles, original private directory names, screenshots, credentials and VM
inputs remain private. The original relocation script keeps its exact command
hash and execution receipt in private operational state for future lookup.

The installed Puck launch-agent bundle separately reports version 1.4.5, with
the running executable mapping the current on-disk image. Its source revision is
unattested by this observation. That service remains running; this source/data
acceptance does not establish an installed v1.4.7 upgrade. Ordinary-user/other
native-platform, SSH/provider, external authentication, signing, installed-rollout
and complete roadmap acceptance retain their separate requirements. In
particular, #762 and #110 remain open.
