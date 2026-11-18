# Native Codex creation factory

The internal `native-create` runner action composes the qualified manager,
default native owner-plan resolver, launch wrapper and durable creation journal.
Go embeds its dependencies and supplies the authenticated owner's storage
namespace. This checkpoint does not connect the public HTTP creation route or
browser controls to that action.

The creation receipt captures source revision `25ac5db`; the factory and
wrapper sources remain unchanged by the subsequent adopted-launch fix below.

Before dispatch, the factory durably publishes a frozen owner plan, minimal
agent definition, exact member request and immutable wrapper bundle. The
bundle directory is named by its source hash; later helper extraction cannot
overwrite a running seat's wrapper. Inspection checks the original plan,
agent definition, bundle and manager identity before replaying the exact
original request and operation ID. Ordinary duplicates consult the consumed
journal without another POST or native owner-observation process. Explicit
inspection can execute the original request if its first dispatch never
arrived; it must not be presented as a read-only status refresh.

The [source-bound receipt](receipt.json) records actual authenticated macOS
ARM64 creation through Codex 0.169.0, Rig 0.7.18 and Node 24. The default native
owner has full access with approval `never`; the factory preserves that policy
instead of silently narrowing it to workspace write. A fresh provider reads a
new challenge, reports its native Rig identity and performs four correlated
file writes permitted by that policy. The initial response times out; explicit
inspection recovers the original enrollment. Duplicate creation and another
inspection preserve the same operation, node and process. Checked native Stop
ends only the selected coding seat, with the sibling retained. Private manager
and tmux cleanup follow the assertions.

The resolver also avoids granting project trust while inspecting settings.
Native `thread/start` with an explicit `cwd` can persist trust, reload a disabled
project layer and start its integrations. The resolver now requests layer
metadata, refuses any disabled project layer before account/thread creation,
sets the private child process's physical working directory and omits `cwd`
from `thread/start`. The reproducible [native trust control](../../../scripts/qualify-codex-project-trust.py)
uses disposable homes and no authentication or coding turn. Its positive
control reproduces native trust/configuration mutation and project MCP startup;
the actual resolver refuses with configuration unchanged, no marker process
and no persisted session. Cancellation controls separately prove that the Go
owner permits cleanup and the resolver reaps its private app-server.

Validation includes 177 helper controls, five strict TypeScript projects,
272 installed lock identities, coordinator Go race tests/vet and whitespace.
An earlier native run checked Stop immediately after its HTTP acknowledgment
and failed that assertion. It remains excluded from positive evidence. The
final run observes process exit within five seconds, before cleanup, and checks
the sibling again. No application assertion deadline was changed.

Remaining work includes the public creation/uncertainty UI, durable namespace
creation and faithful execution-environment composition. The factory currently
passes the native `CODEX_HOME` reference; it does not qualify general environment
equivalence between the service and manager. It must remain unactivated in the
public creation route until that composition is implemented and tested.
Configured MCP tables merge rather than replace: a server added between native
configuration read and thread creation can still start. Enumerated startup
suppression is not a guarantee of inert observation. Selected profiles, named
permission graphs and other providers remain unsupported by this helper.

The [earlier native resume receipt](../codex-native-resume-2026-11-18/README.md)
qualifies provider-process loss using explicit workspace policies, original
conversation/log retention and duplicate controls. It does not compose the new
factory with WebMux UI recovery. Linux/Windows authenticated creation, manager
loss, host reboot, review, CI, audit and release gates remain open. Existing
adopted seats retain their existing lifecycle authority.

## Existing adopted launch policy

The existing local/SSH creation route previously added
`-c sandbox_workspace_write.network_access=true` to every Codex command. It now
supplies only the existing identity prompt and inherits native permission
configuration. Local shell-execution and remote generated-command controls
verify the override is absent. A separate [native control](adopted-launch-policy.json)
uses fresh homes and no authentication or coding turn: a workspace owner with
network disabled remains disabled without the flag, while the removed flag
changes the effective native policy to network enabled. Coordination access
remains subject to the owner's network policy. This change does not convert
adopted seats into native manager-owned seats or qualify authenticated public
UI creation.
