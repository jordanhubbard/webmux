# Private SSH channel integration and live account revocation

This [source-bound receipt](receipt.json) integrates
[#647](https://github.com/jordanhubbard/webmux/pull/647) with the corrected project,
policy, peer-cache, message and reservation parent for
[#630](https://github.com/jordanhubbard/webmux/issues/630).
The provisioning conflict preserves both ordinary/SSH eligibility and current
account authorization. All reviewed parent fixes remain present.

A freshly built Linux ARM64 production helper was transferred with matching
SHA-256. Four current-source native cases use real authenticated SSH, provider
bootstrap, Unix-socket forwarding and the scoped coordination CLI, with a
race-instrumented macOS controller:

- Ordinary startup followed by same-pane reconnect publishes a new private
  channel and lets the retained shell use the replacement locator.
- First-use trust works through a new private known-host file without changing
  the user's host-trust files or weakening a strict selection.
- Refused forwarding preserves a usable shell, refuses the scoped CLI and reports
  onboarding pending.
- A non-admin fixture account first uses the working remote interface. Public
  account deletion then causes its retained remote CLI read and action to reach
  the actual private API and receive witnessed HTTP 401. The original pane PID,
  terminal incarnation and connected state remain; the task is unchanged, ledger
  authority becomes inactive, and the operator account remains usable.

Every case also requires the owned SSH master to exist before source close and
exit afterward. The fixtures select only their transferred helper and private
home/cache/tmux namespace. Their providers and temporary state are cleaned up.
No installed service, helper, user agent or account configuration is changed.
The remote production helper's hash matches again after all four cases.

Validation includes the full Node 24 Go suite, core/client/MCP/session/recovery/
terminal race tests, scoped HTTP API race tests retaining all parent revocation
controls, vet, strict TypeScript and the production CLI/MCP HTTP/WebSocket/restart
contract. Windows ARM64 test compilation passes; it establishes compilation only.
The first three fresh native successes before the account-control extension are
retained separately from the final four-case race-instrumented run.

This proves ordinary SSH shell/CLI channel behavior. Native coding-agent adoption,
password startup, native Mosh/Windows, unexpected tunnel-loss repair, existing
agents lacking MCP, full production controller restart and managed/discovered
workflow acceptance remain separate. Fresh hosted checks and parent delivery
remain merge gates, and the wider roadmap stays open.
