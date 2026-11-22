# Shared heavy-worker authority across WebMux nodes

This [source-bound receipt](receipt.json) covers the
[reservation protocol](../../session-coordination.md#shared-heavy-worker-reservations)
for [#630](https://github.com/jordanhubbard/webmux/issues/630). It does not complete
real coding-agent job acceptance or the full roadmap.

One node's ledger owns its heavy resource. Local and remote contenders use that
same atomic record and fence counter, including differently named receiving
accounts. Separate outgoing/incoming reservation permission, namespace binding,
current verified project context and pinned HTTPS are required. Summary/message
permissions alone cannot acquire a worker. Admission starts no job or provider.

The native run uses macOS ARM64 as source and Linux ARM64 as authority over routed
end-to-end HTTPS. Each owns ordinary PTY/API test children, independent real Git
repositories and worktrees with matching history/remote. Homes, databases,
capabilities, certificates and credentials are isolated. Fixture trust does not
alter OS certificate stores; installed services/user agents remain untouched.

The receiver deliberately loses the first committed reservation reply. The source
retains uncertainty, then retrieves the original grant by status without another
reserve admission. A local authority caller is busy under remote fence 1. The
source renews the original record/fence, releases it, and cannot use its historical
success as a current lease. A new local holder gets fence 2; the old remote holder's
renewal is refused. The existing handoff-reply loss and summary revocation controls
also run. No browser/manual maintenance call is needed. The receiver witnesses
identity proof before bearer credentials on each real TLS connection. SHA-256
verifies artifact transfer, and fixture credentials/processes are removed afterward.

Deterministic/race/HTTP controls cover concurrent local/remote acquisition, source
and authority reopen after reply loss, immutable outcomes, original-fence renewal,
old receipts not inheriting renewal, release/project changes, delayed reply
fencing, distinct receiving-account privacy and refusal, foreign-source namespace,
fresh authority history, source revocation, clock offsets and conservative local
deadlines, separate messaging permission, and legacy-client refusal. Current
CLI/MCP clients require current lease plus expiry and declare the lease protocol;
older clients cannot turn a historical success into work permission. Schema 3
preserves prior keys/history/fences and prevents unsafe downgrade interpretation.

The receipt records exact source, commands, native artifacts and scope. Windows
ARM64 cross-compilation remains compilation evidence, with hosted native gates
separate. These are protocol controls using ordinary process fixtures. Real
independently started coding agents must still demonstrate discovery, overlap
negotiation, acknowledged handoff and actual heavy jobs honoring shared leases.
Managed/SSH and existing-agent onboarding, live notifications, explicit mappings,
scale/retention/backoff, platform custody/distribution and full LAN reconnect
acceptance remain open. No roadmap checkbox is completed here.
