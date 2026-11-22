# Removed peer mappings reject in-flight context

Integration review of [#638](https://github.com/jordanhubbard/webmux/pull/638)
found that a delayed response could populate the cache after maintenance observed
the mapping's removal. Restoring the identical mapping then exposed that old
response as current, before any fresh verification. This also affected a first
response: there need not be an existing cached value to prune. The original
regression fails and remains retained.

The cache now tracks each continuous authorization period separately. Removing a
mapping retires that period even when its first response is still in flight.
Refused, disabled or invalid responses also retire all requests already in flight
for that scope. A response can commit only to its original, still-current period.
Restoring the same configuration creates a new period and requires a fresh
observation. This replaces wall-clock comparisons for refusal ordering while
retaining the existing payload, expiry, concurrency and view limits.

`TestCoordinationLateSnapshotCannotResurrectRemovedMapping` uses controlled real
TLS responses and the existing ordinary PTY fixture:

- Hold the first authorized snapshot response after the receiving API admits it.
- Remove the outgoing mapping and run refresh so the removal is observed.
- Release that old response, then restore the original mapping.
- Require pending context with no snapshot until a fresh fetch succeeds.

The fixed test and the earlier refused-scope regression both pass. Peer/account/
permission/MCP controls also pass under the race detector. The complete Node 24
Go suite, vet, strict TypeScript, frontend lint, all 479 frontend tests, production
frontend build and production CLI/MCP server contract pass. The combined source
retains #635's account-removal and #637's compact-native-option corrections.

The [receipt](receipt.json) binds source and private logs. It retains the original
failed regression and an intermediate compile failure from updating an unkeyed
job initializer. The initializer is now keyed; neither failure is counted as a
passing result. Raw logs remain under ignored `_build`.

This is a local controlled TLS/concurrency regression, not a new two-host native
agent or mDNS result. The original native peer-component record remains in
[the earlier receipt](../coordination-peers-2026-11-21/README.md); it does not bind
the corrected binary. Fresh hosted checks are required on the updated #638 head,
and parent #635/#637 delivery is still pending. #630/#631 and their complete
native/managed/discovered workflow requirements remain open. No roadmap checkbox
is completed here, and no installed service or existing user agent was restarted.
