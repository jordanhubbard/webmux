# Coordination interface provisioning

Local qualification for #669 and PR #672, 2026-11-22. The earlier provider-startup fix released the broker mutex around native process launch but retained coordination interface provisioning under that mutex. Provisioning syncs the ledger and writes/fsyncs access files. A deliberately blocked provisioner therefore blocked inventory of an already running terminal.

Provisioning now uses a separate serial queue and takes a fresh broker snapshot after entering that queue. The broker records the pending generation before provisioning, then rechecks deletion, shutdown and current agent access before starting a provider. Revoked agent creation keeps the established error-state response and starts no provider. The existing post-launch generation/access checks still protect late provider returns.

The regression controls run the new tests against published revision `fea5c3daff851eec9f0891f53c74d07fa85e2ad4` using a Go overlay for the original broker/coordination sources. Both controls fail: a paused provisioner blocks live inventory; revocation during provisioning leaves the returned agent apparently pending. Current source passes the controls plus queued-snapshot freshness, deletion, shutdown and geometry coverage. The five-second scoped-client deadline and request/authority semantics are retained.

Final local checks pass with identical source digests before and after execution:

- Full Go suite.
- Complete session and coordination-client race suites.
- Coordination HTTP API race cases.
- Node 24 real-server HTTP/PTY/restart contract.
- Vet for session, HTTP API and client packages; diff checks.

The receipt records source and command-log SHA256 values. Detailed logs, negative controls and intermediate failed candidates remain in the owner's ignored `_build/coordination-transport669` evidence directory.

This proves the additional broker-lock defect and its controlled correction. It does not establish that this defect caused the earlier intermittent Windows five-second timeout. The earlier diagnostic revision passed Windows server race and unit tests; those observations do not qualify the current changed source. The native repetition chronology below includes both a pass and a subsequent failure with identical runtime inputs; the latest registration correction still requires native qualification, full current-tip CI and main integration.

The initial native Windows repetition workflow `.github/workflows/coordination-native.yml` selected the original peer-claim/startup fixture and seven provisioning/provider lifecycle regressions. It ran each ten times without the race detector, stops on a failing Go test, and requires exactly ten JSON pass events for every selected root test. A successful command with missing tests therefore cannot qualify. Source/PR-head identity, Go/Node/native target metadata, resource observations, test events and verified counts are retained as workflow artifacts. This repetition passed on native Windows AMD64: [run45058272386](https://github.com/jordanhubbard/webmux/actions/runs/45058272386). Its native checkout tree equals candidate `4a837fec43251e87a73ad39b4ecfe91f88ab7ca1`; all80 required root-test events pass, with ten per selected case and no failure events. [The verified native receipt](native-windows-repetition.json) records exact source, native versions, input-tree binding and artifact digests. This is a native loopback two-server/PTY fixture and lifecycle result, with separate multi-host/MAC acceptance still open.

## Pending-registration follow-up

A later repetition on identical runtime/workflow inputs failed on the second original peer-startup case: the child had no automatically provisioned interface. [The retained failure receipt](native-windows-repetition-failed.json) records71 preceding root passes and the actual failure; the earlier80-pass result is not a substitute for this observation. PR #672 returned to draft.

Moving provisioning outside the broker mutex exposed a registration/issuance interleaving: another child’s view or background refresh can observe the pending participant as inactive between separate membership sync and capability issuance. A launch snapshot can also become stale and must not withdraw members absent from that snapshot.

`ProvisionLaunch` now atomically registers only the broker-validated pending participant and issues its interface. It forces pending membership inactive and cannot alter or withdraw siblings. Ordinary `Provision` keeps its active-member requirement. Disabled, revoked, invalid and foreign-node controls still refuse issuance; pending capabilities cannot read/act until an authoritative live broker snapshot admits them. An unregistered participant cannot act and a withdrawn incarnation cannot revive. The broker’s generation, owner, deletion, shutdown and post-launch checks remain in force.

New local checks pass with unchanged source digests in [the registration receipt](launch-registration-local.json). A deliberately split-transaction mutation control fails the interleaving test; it is not represented as a historical source checkout. Coordination fixtures retain debug/error reasons through the test output stream and report interface-variable presence without printing their contents.

The next native workflow selects eleven regressions across coordination, session and HTTP API packages, requiring ten pass events each (110 total) and retaining the first failure. The expanded native Windows job passed all110 required root events on candidate `82fc07116a777f5f6f5b25ad0effcd8f000a110c`: [run45061206001](https://github.com/jordanhubbard/webmux/actions/runs/45061206001). The native checkout has the identical full tree; [the verified receipt](launch-registration-native.json) preserves all eleven counts, source/tool versions and artifact hashes. This replaces reliance on the earlier80-pass result for registration qualification. Full current-tip CI and main integration remain pending.
