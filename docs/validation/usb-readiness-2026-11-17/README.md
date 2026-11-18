# Exact-payload readiness for the disposable USB fixture

[#459](https://github.com/jordanhubbard/webmux/issues/459) follows native job
134594632622 at source `52a8527`. The held-reply replacement control discovers
its serial-matched vhci_hcd sysfs entry, then opens `/dev/sda` before the node
is available. The open raises ENOENT. Artifact 14399524652 retains the raw guest
log, exact source/module hashes and the refused corrected-held result. Source
acquisition/checksums and the unchanged unpatched negative control passed.

Enumeration now declares readiness only after re-discovering the current fixture,
opening it and reading the exact expected bytes inside the existing poll. It
retries ENOENT/ENODEV disappearance on the next poll. Permission errors, unexpected
I/O (including ENXIO), short/corrupt bytes and an empty expected payload remain
hard failures. No cached device path is carried into another readiness attempt.
Detach detection still uses sysfs absence rather than treating unreadable data
as successful detachment.

The initial, held-reply replacement, twenty-cycle direct and WebMux/SSH import
paths share this readiness check. Kernel patches, module/source checksums, the
held native reply, exact data/write/cleanup oracles, thirty-second poll and
outer guest deadlines are unchanged. This is fixture synchronization, not a
kernel repair or physical-device qualification.

All 29 local controls pass: ten sysfs/readiness cases, eight source-cache cases,
three apt controls and eight native-qualification admission controls. The new
cases reproduce absent-node selection with a real temporary path, then verify
readiness only once exact bytes exist; they also cover fresh rediscovery,
disappearance during open/read, wrong/short payload and hard-error refusal.
These portable controls do not establish native USB acceptance. The first apt
unit invocation used the wrong discovery directory and discovered no tests; the
correct tests directory passes all three cases, with both logs retained.

[Source bindings](source-manifest.json) retain the failed native artifact and
local log hashes. Native corrected-held, direct-cycle and real SSH results
remain pending until the exact candidate is run. No audit or native acceptance
gate is weakened; #429 still blocks merge/release.
