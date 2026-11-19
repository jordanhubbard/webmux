# Preserve the error after a failed preparation save

Ordinary-local creation banks its original entry and recovery key, prepares the
launch, then saves the result. If only the preparation save fails, no process
starts. Previously, a successful final save could retain `connecting` despite
that failure. The reviewed fix from e19b88b is adopted as 1801056: mark the
retained entry `error` and update its timestamp. The original ID, key and scope
remain available. Save-before-launch and configured restoration behavior are
unchanged. See the [source-bound receipt](preparation-save.json).

The real-store regression fails the unmodified b07e4db production code at the
persisted `connecting` state, with zero launches. The fixed test passes twenty
race repetitions on macOS. Full session/API race suites and Windows-target vet
pass. On actual Windows AMD64, the same parent-production overlay fails at the
expected boundary; the fixed preparation, initial-save and post-launch-save
controls each pass twenty repetitions: sixty passes, no failures or skips.
No Go runtime overrides were present. Both executed images have zero Restart
Manager holders after completion.

These tests use the real store and filesystem with mocked processes. The new
case exercises the preparation/final-save sequence directly, not a fault
injected through HTTP Create. Explicit retry starts the same session once in
this sequence. This does not change automatic restoration policy, execute a
native coding CLI, or requalify earlier native application receipts. The first
local overlay setup used an incorrect relative path and ran no tests; that
attempt is retained and excluded from qualification. The corrected negative
control runs the actual regression and fails as expected.

The original authenticated Windows evidence is also available for review
through the established private roadmap handoff. All 146 frozen raw files
match the original manifest SHA-256
`f141ca92e790a12afae23c72e554f026e5b37fa8e56dfe3f0f0bec4afeeeacc5`.
The private archive's members were read back and checked against that same
manifest. Its digest and retrieval field are recorded in
[the packaging receipt](raw-archive-retrieval.json). This is archive/digest
verification; independent semantic review remains required. No native case was
rerun, authentication storage copied, or raw private evidence committed.

Exact-head CI, review/integration and full #318/#438/release acceptance remain
open. The original native qualification retains its declared source and limits.
