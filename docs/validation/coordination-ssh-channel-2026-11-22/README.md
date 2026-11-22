# Private ordinary SSH coordination channel

Issue [#630](https://github.com/jordanhubbard/webmux/issues/630) remains open.
Ordinary key-authenticated SSH sessions now receive a scoped CLI channel during
normal provider bootstrap. The controller's capability stays in private files
and encrypted SSH stdin. The remote endpoint is a private Unix socket forwarded
to the source's loopback-only agent listener. No remote TCP coordination listener,
administrator credential or federation credential is installed.

Before publishing access, the receiving helper queries the actual forwarded API
and checks the complete subject identity. A refused probe preserves existing
access. A lost installation acknowledgment may follow a committed file, so the
forward remains until its owning run closes. Each run owns a distinct SSH control
path; shutdown closes that master. Provider ownership remains with the existing
session lifecycle implementation.

## Native evidence

`TestCoordinationSSHNativeBootstrap` runs a real normal SSH broker session from
native macOS ARM64 to Linux ARM64. It uses production bootstrap, authenticated
RPC, SSH forwarding, provider, directory resolver and scoped client code.
The test adapter selects only the transferred helper and an isolated fixture
home/cache/tmux namespace; actual SSH authentication/encryption is retained.
Only the fixture's providers and cache are cleaned up. No installed server,
helper, existing agent or account configuration is restarted or replaced.

The [receipt](receipt.json) binds code, executables and private raw logs. Successful
cases prove:

- Startup configures and verifies the channel before provider attachment.
- The remote ordinary CLI reads its scoped view and updates its task under the
  correct verified execution-host repository and broker subject.
- Reconnect uses a new private control connection while retaining the exact
  remote pane PID; that existing shell uses its newly published locator.
- A fresh private known-host file works through the normal SSH first-use policy.
  Fixed background RPCs remain strict; bootstrap never weakens an original
  strict selection. The fixture does not edit the user's known-host files.
- Refused forwarding leaves a usable ordinary shell, no working scoped client
  and pending onboarding rather than a successful interface claim.
- The owned private SSH master exits on source close. Capabilities do not appear
  in the retained terminal output.

## Original failures and controls

The first two refusal attempts fail their original startup observation gate and
remain recorded as failures. With capture off, shell-directory output can happen
before attachment. The new authenticated read-only provider query fixes that
missing observation path without enabling history capture. Its initial
`=session` tmux selector then returned an empty pane format with exit code zero.
A native controlled probe demonstrated that the exact current-window selector
`=session:` returns the directory. The corrected source passes the refusal and
reconnect cases. The original 35-second startup wait was not extended.

Race controls exercise private socket permissions/type, complete subject checks,
refused installation preserving the original file, fixed forwarding destinations,
host-trust policy, authenticated bounded directory queries and live-only metadata
with capture disabled. The complete Node 24 Go suite and vet pass. Windows ARM64
test compilation passes; compilation is not native Windows acceptance.

This qualifies ordinary SSH shell/CLI transport. Native coding-agent adoption,
password startup, native Mosh/Windows, unexpected tunnel-loss repair, existing
agents without MCP, full controller restart and durable claim/message/reservation
continuity, managed providers and broader native lifecycle/cache cleanup remain
open. The observed shell reconnect is not proof of those broader requirements.
Multicast discovery/enrollment is separately unfinished under #631.
