# Independent review of retained Windows evidence

This review reads the original frozen evidence; it starts no native job and
does not promote historical results to current combined-source acceptance.

## Ordinary local lifecycle (#541)

The original b2b9fce archive matches SHA-256
`d1c7749305b4ca881b497df278a872479679680317b0b081a197c44622882aad`.
All 146 files match sizes and digests in manifest
`f141ca92e790a12afae23c72e554f026e5b37fa8e56dfe3f0f0bec4afeeeacc5`.
All 147 safe tar members byte-match the retained tree, including the manifest.
The 20 retained implementation/test source files match published b2b9fce.
Documentation snapshots are excluded from that implementation-source comparison;
they are retained archive content, not executable source bindings.

The [semantic audit](local-lifecycle.json) checks actual API response sequences,
original selections and operation IDs: Hide retains connected work; live DELETE
returns 409; a lost successful Stop response retains the stopped record; duplicate
requests return the original stopped operation before and after controller
replacement. An uncertain Stop returns 202, preserves access, and disables
persistent relaunch. Inspect-only requests keep the original selection, first
report uncertainty, then confirm late original completion before deletion.

Eleven PID/creation/image identities and two provider identities survive the
controller replacement. The corrected two-viewer receipt has no pre-Stop socket
closures, nonzero output on both viewers, preserved shell output and original
selection through Hide/restore, confirmed Stop and subsequent absence. Fourteen
reported log-prefix checks match; decoded embedded bytes also pass their hashes
and actual prefix comparisons. The native journal contains four completed turns,
one real tool call with matching exit-zero output, and the recalled marker.
Final process-object and Restart Manager checks confirm the recorded cleanup.

This clears the previously missing independent raw-archive review. It does not
close #541 or qualify the current composition, service/MSI upgrades, other
accounts/policies, authenticated ARM64, or the interrupted persistence/dispatch
window. The original legacy Windows sandbox evidence does not clear #538.

## Application startup (#543)

The [application review](application-startup.json) verifies all 15 frozen-file
digests, published d4b3e45 test-source bindings, original application-image digest,
byte-identical renamed gate, Playwright result/attachments, and cleanup receipts.
The private Windows application test passes once in 8184 ms with zero skips,
retries or flakes. It checks TTY child output, original selection, authenticated
Stop, child exit before its independent expiry, and duplicate Stop.

The original failed hosted log at that same d4b3e45 source reports owner creation
error 5 and a bootstrap immediate job that disallows breakaway. The observation
describes that job; ancestor restrictions and all potential access-denied causes
remain unproven. No protection setting, job flag or deadline changed.

These are the owner's historical gate and application, distinct from the
canonical 5b7ea984 application/gate whose hosted run also fails owner startup.
This review neither adopts the proposed gate-discovery separation nor marks
canonical native acceptance green. Release admission still needs actual
source-matched native application evidence.
