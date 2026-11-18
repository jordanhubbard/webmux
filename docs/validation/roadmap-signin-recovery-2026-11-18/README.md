# Composed sign-in, USB and recovery inputs

The USB roadmap candidate from #496 now also contains the owner-requested
password Return accelerator (#480), the scoped Windows installed-service
qualification (#465), and the standalone canonical APT guard/platform receipt
follow-up (#493). All four pinned inputs remain ancestors.

Only LoginPage changes production behavior relative to #496. The entire Go
server tree and package-lock blob match that parent. The two recovery document
conflicts were resolved by retaining both the actual installed-service evidence
and the separate ARM/native-component limits. The imported Windows receipt is
unchanged: it qualifies its original main8f5d322 artifact and never claims fresh
combined-source authenticated recovery. The APT source matches the corrected
combined guard; its newer platform receipts are retained.

[Source-bound receipt](receipt.json): all 418 frontend tests in 42 files, five
strict TypeScript projects, frontend lint and full application build pass.
An actual sandboxed Chromium156 instance signs in to the built native Go server
in local-auth mode by pressing Return in the password field. Exactly one login
POST occurs and the workspace appears. Under that authenticated owner, the
actual USB UI creates and pairs a lease, observes transport status, requests
Stop and waits for a final release report. The helper reporter is synthetic;
this browser check does not claim a native USB device or coding conversation.
The browser and its private controller are closed afterward.

The last receipt commit changes documentation only. All hosted combined-head
CI, native simulation and packaging checks remain required. Audit #429, native
Windows runtime/profile-cleanup investigations, credential-backed notarization
and the broader roadmap acceptance are still open. No main merge, release or
whole-roadmap completion is claimed.
