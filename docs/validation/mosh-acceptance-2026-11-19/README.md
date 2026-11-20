# Mosh browser control and provisioning acceptance

This audit completes the scoped requirements of #468 and the provisioning-evidence
follow-up #561. The [receipt](receipt.json) keeps each execution at its actual
source. Broader #110 identity-provider, physical-host, hardware, native-SSO,
client-certificate, assistive and Windows Mosh acceptance remains open, together
with the rest of the roadmap and release gates.

| Requirement | Verified evidence |
| --- | --- |
| Deliver automatic launch with recording disabled | The helper queues a sealed request on the execution host. The existing authenticated SSH policy RPC carries it even with recording off; it creates no new listener or polling loop. All 20 original native Linux samples use real SSH/Mosh/tmux and the actual browser, refuse SSH fallback, show the automatic sign-in panel and complete callback delivery to the originating CLI through an observed SSH worker. |
| Keep URL contents out of terminal/activity/recovery metadata | Current race controls verify domain-separated AES-GCM, session/key/ID/expiry/content authentication and rejection of malformed, oversized, cross-session or expired envelopes. Native samples inspect the host cache for sealed pending data with recording off and observe no launch URL in terminal output. |
| Keep only the latest five-minute request | Host tests replace the pending request, preserve it against an older or foreign acknowledgement, clear only the exact request and prune expiry. Controller controls translate host time, including clocks a day ahead/behind, and preserve the original delivery lifetime. |
| Persist owner-checked acknowledgements | Current controls cover owner rejection, failed-save refusal with pending state retained, reconnect and reconstruction without another successful host reply. Native samples dismiss, observe host clearing, reload and verify that the acknowledged panel does not replay. |
| Preserve Local/SSH behavior and truthful fallback | Disabled control refuses without enabling activity capture. The ordinary PTY packet/parser and shell-startup controls remain, including sensitive-output removal, malformed/bounded packets and profile PATH overrides. Existing providers are not restarted; older helpers retain the printed-link fallback. |
| Require actual ownership and cleanup | Each native receipt confirms provider, Mosh server and private SSH daemon exit. The fixed 20-invocation workflow uses a fresh backend per sample, zero retries, the original 120-second test and 12-minute job limits. Current nine tmux controls require a later absent/refused connection after a reset; a reset itself never proves shutdown. |

## Source and native evidence

Original `0056000940ec0dbc96a010c3b88d349759c51c48` workflow 44541533377 passes
20 separate one-test invocations. Its source-bound artifact 14538168257 and all
63 uploaded members are rehashed. Each distinct trial has all ten native flags
true, and the raw job log independently records 20 invocations and 20 passes.
This is owned loopback SSH/Mosh with a synthetic identity endpoint and CLI;
it is not external-provider or physical remote-host authentication acceptance.
The diagnostics ZIP does not independently attest the application executable.

The listed protocol, host cache/RPC, broker, viewer/acknowledgement, callback,
terminal helper, AuthHandoff UI and test sources remain byte-identical to 0056000
at audit baseline `34c3030`. The session model differs only by a new RigIdentity
project field; literal removal recovers the original file. The three auth-handoff
API entries are unchanged. Newer BrowserCompanion read-page-generation behavior
is outside this automatic AuthHandoff fixture and is not qualified by it.
Current native macOS Go race checks pass 22 top-level tests, 41 including
subtests, without failure/skip; all nine private-tmux controls pass.

The unchanged-parent native negative still fails the automatic-panel assertion
at its original 15-second deadline. The old 895933e cohort retains 19 passes and
its trial-5 cleanup failure with ECONNRESET. Current probe controls and later
native passes support the specific probe correction; they do not establish the
cause of the earlier masked 45151c3 cleanup failure. Those failures stay failed.

## Provisioning evidence and disposition

Original run 44521398749 fails APT with select/11 before any Mosh sample, then
fails to write inventory because its parent directory is missing. Changed-source
run 44522389452 preserves the same APT failure while successfully writing and
uploading install output (1,756 bytes), initial worker limits (1,350 bytes) and
native inventory (3,297 bytes). The original stdout and artifact are verified;
initial limits do not establish peak pressure or the network failure's cause.

The failure path creates its owned output directories before setup/inventory.
A current exact-install-block control uses a failing sudo stand-in: update exits
100, later install commands never run and output is retained. The always-run
actual local version inventory succeeds, then refuses a repeated output path
without changing its first bytes. This is a failure/inventory control, not
native APT or Mosh acceptance. Inventory probes have their existing individual
time/output bounds. The diagnostic job remains bounded to 12 minutes; its tee'd
install log has no separate byte cap, and no byte-bounded-log claim is made.

Historical no-retry endpoint observations subsequently return 200. On the same
published db82695 source, original APT 44526242446 passes signed-positive,
vendor-exclusion and untrusted-key controls plus all 22 Python controls on both
Ubuntu 22.04/24.04; original recovery Mosh 44526859801 then passes its first
20 actual native samples. Those logs and source-bound ZIPs remain distinct
from both failed setup runs. The later 0056000 Mosh and APT qualification covers
its later helper composition separately. This dispositions the observed setup
failure and verifies the evidence repair; it does not attribute the precise
historical select/11 cause or assert a code, mirror, trust or timeout repair.

No native assertion/deadline is relaxed, no failed native sample is retried for
green, and no new hosted workflow, account copy, host service/protection change
or release action is used by this audit.
