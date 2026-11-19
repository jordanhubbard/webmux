# Checked local Rig seat Stop

The #438 candidate adds review and explicit confirmation for a running local seat launched by native Rig. Native enrollment lineage, rather than WebMux's private launch record, determines whether the review action is offered. Adopted seats remain ineligible.

The adapter admits only CLI 0.7.18 (3bf87c62) and the matching stamped daemon build. It corroborates the health PID/effective UID against the local listener, OS owner, process start, Node executable identity and daemon entry point before review and before dispatch. Linux uses the kernel's `/proc/<pid>/exe` link; macOS uses the process executable name. Only a process fingerprint is retained. A basic loopback tunnel, a proxy process, another account, an unstamped build, changed process or changed socket refuses without lifecycle dispatch. These are accidental-routing and process-provenance controls within the service account's trusted environment, not authentication against a malicious same-account process.

The first hosted Linux checked-Stop run at `6d178b5` refused its initial review
with `local_manager_unverified` (run 44263510039, job 134662317906).
Official Node 24.21.0 on Linux reproduces `ps comm=MainThread`, which the first
adapter incorrectly required to end in `node`. The follow-up uses the kernel
executable link rather than admitting a thread name. Controls cover MainThread,
non-Node/deleted/malformed executable links and executable replacement after
review; the MainThread control fails against the parent adapter. Hosted native
Linux acceptance of the follow-up remains pending. All 121 helper controls,
strict TypeScript, affected Go race/vet and the native macOS browser case pass
for the executable-identity fix. The original hashes in `sourceSHA256` identify
the initial `6fa9435` snapshot; `linuxManagerIdentityFollowup` records the newer
adapter hash and its separately scoped checks.

The owner reviews the exact local tmux seat, native node/enrollment, reason and effect. The checked HTTP operation receives the original expected IDs unchanged. Native guards remain responsible for binding/occupant changes, typing protection and terminal effects; no unchecked fallback or direct tmux kill is used in the product path. A detach phase never becomes a claim that a kill occurred.

Before any lifecycle dispatch, an exclusive enrollment-wide claim is synced with its file contents and directory links through the private storage root. Reviews and results are owner-scoped. Another admin, a changed daemon port, controller reconstruction or an expired review cannot bypass an uncertain claim. Completed results are immutable and duplicate requests return them. Only a positively confirmed no-effect refusal permits a new explicit review. An interrupted claim without a completion remains uncertain.

On native macOS ARM64, published Rig/tmux and the real browser pass actual native replacement-after-review refusal, adopted-seat refusal, explicit confirmed termination of only the selected replacement, sibling/adopted survival, retained viewer/layout/history and duplicate consumed receipt. The fixture uses terminal/sleep processes; it does not establish authenticated coding recovery or remote provider termination. Source and private receipt hashes are retained in `results.json`. The earlier raw native probe independently exercises the manager's checked contract; unit protocol controls cover effect phases and lost/inconsistent responses.

All 118 helper controls, all 402 frontend tests, affected Go race/vet, strict TypeScript, frontend lint, build and YAML parsing pass. A separate Linux CI lane installs the exact published manager without replacing the legacy compatibility lane, asserts its build before running the native case, and retains its dependency lock. Hosted results are still required.

Full #438 remains open: WebMux-created adopted launchers need supported native ownership conversion with original policy/host semantics, and real SSH-relay acceptance remains separate. Native Stop here acts on the verified local tmux seat; jobs outside it may continue. The optional manager's retained audit snapshot has one moderate @hono/node-server advisory (GHSA-rmxm-3fg6-px4f), propagated across three vendor package records. That component review remains under #91, separately from first-party audit #429. No downgrade, override, audit suppression, installed manager upgrade, runtime-protection change or release mutation is included.
