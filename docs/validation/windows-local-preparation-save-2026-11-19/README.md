# Local preparation save failure

Implementation `e19b88b` repairs the additional pre-launch save boundary in #541.
After Create banks a session/key, a transient failure in startLocked's preparation
save could leave the final saved session connecting even though no job launched.
That failure now sets error and an updated timestamp. The original session, host
scope, key and access record remain; no job starts until explicit continuation.

The [receipt](receipt.json) binds a deterministic real-store fault sequence:
bank the entry, block the intervening preparation write, restore storage for the
final save, then inspect durable/public state and explicitly reconnect. The final
test fails on parent43d at persisted connecting, passes twenty race repetitions
with the fix, and proves one retry starts the original session with its original
key. The affected session/API race suites and macOS/Windows vet pass. Initial-save
and post-launch-save controls remain. This is broker-level evidence, not an
injected HTTP Create fault or native Windows conversation qualification.

The first positive test incorrectly expected connected before fake PTY output;
its corrected retry assertion follows the existing connecting-with-live-run
contract. That test-authoring failure remains retained alongside the actual
failing-before storage control. No case deadline, assertion from a pre-existing
test, original-key persistence rule or lifecycle ownership is relaxed.
