# SSH reconnect integration and project observation ordering

This integrates [#649](https://github.com/jordanhubbard/webmux/pull/649) onto
corrected [#648](https://github.com/jordanhubbard/webmux/pull/648), preserving the
account-revocation control and earlier policy, reservation and project fixes.
It also addresses [#651](https://github.com/jordanhubbard/webmux/issues/651).
Full [#630](https://github.com/jordanhubbard/webmux/issues/630) acceptance remains open.

Remote project observations now include the broker's live run generation and
complete authenticated activity in their cache namespace. The generation is
private, excluded from serialization, and checked before and after repository
RPCs. Logical session identity and the retained agent capability stay stable
through reconnect. An older lookup cannot overwrite a later-started lookup's
result. Pruning and global policy withdrawal invalidate pending results; global
withdrawal also invalidates refreshes prepared before the policy change. Each
current failed lookup still clears its own verification, and remote expiry
remains 25 seconds. Network/Git work remains outside the cache and broker locks.

Deterministic controls failed with the original cache key and unconditional
commit behavior, extracted into the same begin/finish methods used by production.
They demonstrated cross-run cache reuse, an older failure clearing a fresh result,
an older success replacing it, and prepared/in-flight work restoring withdrawn
context. The baseline patch and failures are retained privately. The corrected
controls pass, including all three withdrawal paths (public policy API,
maintenance and direct refresh), authenticated activity changes, and rejection of
an old run snapshot before starting network work.

The native fixture now holds the original input stream open through completion
of the second turn, then observes the original launcher, actual Claude and stdio
MCP PID/start-time identities again. It requires verified project context beside
the second task before retaining that ledger observation. These are stronger
observations within the existing deadlines. The independent audit requires the
completed-turn identity receipt and rejects JSON booleans used as process IDs.

The first current-stack native test passed, but its independent audit failed:
the second source-ledger observation lacked verified project context. That failed
audit is retained unchanged. The cache defects above are independently reproduced;
the exact interleaving responsible for that historical native failure was not
captured and is not claimed as proven.

After the fix, a fresh macOS ARM64 to Linux ARM64 run passed in 21.54 seconds
with a race-instrumented controller and a SHA-256-verified production helper.
Real Claude Code 2.1.319 used the ordinary SSH alias and dynamic MCP interface
under the explicit two-tool approval profile. The same conversation made five
actual calls across two successful turns. The old SSH master stopped, the pane
and all three native process identities survived through the completed second
turn, and both source-ledger tasks carried verified repository context. The
independent audit passes. Seventeen altered-evidence controls reject missing or
changed process identities, boolean PIDs, stale control/pane evidence, changed
conversation, missing results, invalid turn ordering, reused request ID, false
success, and missing task/project verification. These controls edit private
copies of receipts; they do not run unapproved native tools.

The [receipt](receipt.json) records source, executable and evidence hashes and
check outcomes. Raw native conversations remain private under ignored `_build`.
Existing authentication is reused in place without credential copies. Only the
fixture's own tmux namespace, private socket cache and SSH masters are operated;
installed services, other agents and native account policies are untouched.

This qualifies planned SSH reconnect between completed native turns. Unexpected
tunnel loss, interrupted delivery, production-controller restart and complete
durable claims/messages/reservations lifecycle, managed/existing-agent onboarding,
password SSH, Mosh/Windows and discovered/enrolled two-controller acceptance remain
separate gates. No roadmap checkbox is completed by this result.
