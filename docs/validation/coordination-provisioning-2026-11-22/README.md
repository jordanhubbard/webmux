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

This proves the additional broker-lock defect and its controlled correction. It does not establish that this defect caused the earlier intermittent Windows five-second timeout. The earlier diagnostic revision passed Windows server race and unit tests; those observations do not qualify the current changed source. Native Windows CI, main integration and #669 completion remain pending.

The native Windows repetition workflow `.github/workflows/coordination-native.yml` selects the original peer-claim/startup fixture and seven provisioning/provider lifecycle regressions. It runs each ten times without the race detector, stops on a failing Go test, and requires exactly ten JSON pass events for every selected root test. A successful command with missing tests therefore cannot qualify. Source/PR-head identity, Go/Node/native target metadata, resource observations, test events and verified counts are retained as workflow artifacts. This additional repetition remains pending actual Windows execution.
