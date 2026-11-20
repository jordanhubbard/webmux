# Native pending recovery through controller reconstruction

[#617](https://github.com/jordanhubbard/webmux/issues/617) meets its scoped native acceptance at clean source `4989e16b59e91924d74dd8e83c5fbfa669f6b179`. Two actual native Rig0.7.18/Codex0.169 conversations complete their canary tools, then both original-conversation attachment-loss recoveries pass across controller SIGKILL. A third controller crash preserves consumed replay. The [receipt](receipt.json) binds the application, driver, native journals, immutable operations, log bytes and archived evidence. **Full roadmap and release acceptance remain incomplete.**

## Actual interruption boundaries

| Boundary | Witness before/after controller exit | Reconstruction result |
| --- | --- | --- |
| Broker request banked before native dispatch | The prepared native intent has the same bytes before and after SIGKILL. Dispatch claim, environment acknowledgment and confirmation are absent even after the old controller has exited. The claim is immutable and precedes native POST, so this is not merely a browser timing inference. | Restart with the same private home and reload the browser. The original UUID/body is recovered; ordinary replay returns202/pending. Deliberate Check completes that original operation. |
| Environment handed off, before confirmed completion | Immutable dispatch claim and environment acknowledgment are present and unchanged across SIGKILL; confirmation is absent. The source-bound driver additionally checks a not-ready owner row before interruption. The audited immutable boundary is handoff-before-confirmation. | The same original request survives restart/reload; ordinary replay remains pending. Explicit completion uses the original native key and conditional identity checks. |
| Consumed request | Confirmation already exists. | After another SIGKILL/restart, the same request returns200/consumed and all four native ledger records remain byte-identical. Both coding processes survive this final controller replacement. |

The driver verifies each old controller's actual SIGKILL termination and PID absence before starting its replacement. Four distinct private controller processes form the three-interruption chain. A fresh controller has no prior in-memory broker job; persisted original requests and the native owner's keyed operation remain the authority. Startup may reattach a surviving verified original; that is distinct from dispatching another native Resume or consuming the pending request.

## Complete native workload and audit

Every original line/assertion of the prior two-seat/two-loss driver remains in order, apart from its descriptive scope. The expanded case preserves actual initial tools, viewer reload/reconnect, offscreen reveal, occupied-cell minimize restore, detach/reopen, both original-conversation recalls without tools, sibling identity, retained log paths/prefixes, no duplicate viewers and ordinary PTY input `xy`. It adds the crash boundaries and consumed reconstruction; the original10second response observers, six-Check limit and660second outer bound remain unchanged. The server worker budget stays45seconds.

Both native recoveries finish using3and2explicit Checks. The maximum recovery response is4.679seconds. Native journals independently verify exact original conversation/project identity, each successful initial tool and each after-loss remembered canary, with original-user anchors and no subsequent model tool calls. The recovered seat uses three successive native processes; the sibling remains the original process. The owner records four native sessions, exactly two native requests/dispatch claims/confirmations/environment acknowledgments, with ledger files0600. The prepared request bytes and frozen browser bodies match through reconstruction.

The same recovered viewer retains one transcript filename across both loss/recovery cycles and controller reconstruction, with unchanged prefixes and appended output. The final consumed restart changes neither coding process nor ledger bytes. All42bound SDK files, application sources and intended read-only/on-request project policy remain unchanged. The native account is authenticated in place. Temporary-project trust is reviewed explicitly; this case performs no additional policy-enforcement probe.

The private WebMux endpoint is loopback auth-none. Ordinary PTY input is a control, not proof that its exec process survives controller death. No host reboot, installed-service replacement, SDK upgrade, account/protection change, auth-store copy or hosted job occurs.

## Retention and integration

A separate journal/log/PID/ledger audit passes. The private controller and every process referencing its qualification root are absent; scoped open-file inspection is empty, exact tmux sockets have no server, and the owner environment channel is absent. Selected source, executable, driver, native journals, operation records and log bytes have complete private SHA-256 readback. Whole homes, auth stores, environment values and native databases are excluded; only selected controlled database rows are exported. One rejected driver-preparation invocation used the wrong cwd before any native work; its record remains retained.

Current main's intervening USB qualification/workflow/documentation changes leave the entire application tree and ordinary PTY fixture byte-identical to the executed source. This is source-scoped application reconciliation, not a fresh execution of every current-main qualification. Node syntax, Python compilation, original-driver admission, receipt/source/hash/link/privacy and whitespace checks pass. No product code changes were needed.

The earlier [two-loss response/log qualification](../native-recovery-pending-2026-11-19/README.md) and all original failed reports retain their original outcomes. #318/#438 still require their remaining host-reboot, policy/provider/platform and package-migration coverage. Full #603/#464/#469 and the separately policy-blocked peer challenge/ACK remain open; this case grants no permission escalation or release acceptance.
