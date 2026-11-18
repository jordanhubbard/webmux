# Combined native lifecycle and federation candidate

This candidate extends the roadmap follow-up integration `45151c3` (PR #482)
with checked Rig Stop and its destination/socket identity prerequisites
(`#474`, `d871ac6`), the real SSH relay qualification (`#477`, `1302d7c`),
visible-terminal federation assertions (`#481`, `2f9a13d`), and renderer
capability negotiation (`#486`, `95b8327`). All input commits, the prior nine
follow-up inputs and the four original roadmap integration inputs remain
ancestors. Three explicit merges compose the candidates without conflict.
`source-manifest.json` binds the combined source at `4deeab7` to source trees,
file hashes, input ancestry and the checks below. This receipt adds only docs
after that source snapshot.

The combined source passes the full server, retained VT and Ultraviolet race
suites and vet; 414 frontend tests; 123 helper tests; all five strict TypeScript
projects; frontend lint; and the full application build. Eleven native-inventory
unit controls pass. The additional Linux-only guacd inventory test is explicitly
skipped on macOS, not counted as native Linux evidence. Windows AMD64 HTTP test
compilation, Windows ARM64 server compilation and Linux AMD64 server compilation
also pass; cross-compilation is not native execution.

Seven real-browser specs pass fourteen cases with no skips against the native
Go server, isolated published Rig 0.7.18 and private tmux ownership: Agent
Deck/Rig coexistence, dashboard/discovery and messaging, activity, notifications,
checked Stop, actual Mosh sign-in handoff, and the full federation lifecycle.
Synthetic agent and identity fixtures do not qualify authenticated coding
conversation recovery. Checked Stop retains stale/adopted refusal, sibling
survival, durable duplicate consumption, viewer layout and retained history.
Mosh retains all ten native handoff and owned cleanup receipt checks.

The separate relay driver passes all twelve checks using actual local SSH to
a labelled, disposable Linux ARM64 SSH/tmux container. Stopping the selected
local relay exits its original local SSH PID while retaining the original
remote persistent PID/executable and sibling. Duplicate use is consumed, and
viewer layout/history remain. Only this fixture's container/image and generated
client key are removed. This is a synthetic persistent sleep job, not native
coding recovery or physical-host failure qualification.

Independent xterm qualification passes all 1,356 browser cases. All 1,353
retained replay receipts compare equal at initial attachment, continued output
and recapture, without replayed query replies or the snapshot parser profile on
the reference. The 259 bare-reference receipts additionally retain unmodified
reference methods. These portable comparisons do not replace combined-source
native ConPTY captures. Worker versions and receipt hashes are in the manifest.

Combined-source hosted CI, Packaging, Windows renderer/ConPTY, federation and
startup qualification are pending. Earlier candidate runs are scoped evidence,
not combined-source success. Dependency audit #429 still requires actual
remediation. Windows profile cleanup #473 and retained Windows runtime #450
remain unresolved. #487 records a separate Linux provisioning failure: an
unneeded preinstalled Microsoft APT index returns HTTP 429 before native desktop
and relay steps. No failure is suppressed, retried as a repair or treated as a
successful skipped qualification. No merge, release or complete-roadmap claim
is made by composing this candidate.
