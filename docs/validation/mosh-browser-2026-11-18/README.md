# Automatic browser launch through Mosh

The [current acceptance audit](../mosh-acceptance-2026-11-19/README.md) reconciles the original native executions with current source and retains all earlier failures. Pending-at-the-time statements below remain historical evidence.

[#468](https://github.com/jordanhubbard/webmux/issues/468) implements the remaining
Mosh launch feature under #110. New Mosh providers enable a browser control path
in their shell helpers. A helper seals its latest pending URL in the private
execution-host cache; the existing authenticated SSH recovery RPC carries the
request to the controller, including when logging is disabled. No additional
listener or polling loop is created. Ordinary Local/SSH launch packets retain
their existing PTY path.

AES-GCM uses a domain-separated key derived from the existing per-session secret.
The authenticated envelope binds the session, request ID and expiry. It admits
only the existing bounded HTTP(S) URL policy. The host applies the five-minute
delivery window; its authenticated observation time translates remaining life
to the controller's clock. Expired or malformed requests do not open a panel.
Pending ciphertext is removed on acknowledgement or the next policy sync after
expiry; this is a delivery window, not a secure-erasure claim.

The controller saves only an opaque acknowledgement ID before clearing its
pending request. Existing policy synchronization sends that acknowledgement to
the host. An old acknowledgement cannot remove a replacement request. Durable
acknowledgement survives controller reconstruction even without another host
reply. A failed save retains pending state and reports that acknowledgement was
not remembered. Transport reconnects retain an unacknowledged request. Existing
providers are not restarted; create a new Mosh terminal after updating the
terminal-host helper. Older helpers retain manual link clicks.

## Local qualification

The final full server race suite and vet pass. Controls cover envelope/key/session
authentication, metadata/content tampering, bounded/invalid URLs, expiry,
replacement and exact acknowledgement, recording-off delivery, idempotent host
cleanup, owner isolation, failed persistence, viewer/transport reconnect,
controller reconstruction and host clocks one day ahead or behind. Strict
TypeScript, frontend lint and the production frontend/helper build pass.

Eight real macOS ARM64/Chrome browser cases pass. The new case starts an owned
loopback OpenSSH daemon with private keys/trust files, an isolated host home/cache
and a real native Mosh server/client plus tmux provider. It refuses SSH fallback,
verifies actual PTY input, then runs the existing native CLI callback fixture.
The helper automatically opens the sign-in panel with logging off. The pending
host cache contains sealed data, and terminal output contains no launch URL.
Viewer reload retains the pending panel. Callback delivery through an actually
observed SSH worker completes on the originating CLI; dismissal clears the host
request and survives another viewer reload. Owned provider, Mosh server, SSH
daemon and listeners exit. The other cases retain Local device/callback sign-in,
explicit broadcast recipients, mode-query/editor keys and mouse routing across
broadcast/restart/selection.

On unchanged parent `8e90c3b`, the same final native Mosh case fails because no
automatic sign-in panel appears within its 15-second assertion. Actual Mosh/PTY
startup succeeds and owned cleanup passes. Only the new test and its isolated
SSH runner setup are overlaid on the parent; application source is unchanged.
Later parent assertions are not reached, so their false receipt flags do not
establish logging or callback failures. [Source/log bindings](source-manifest.json)
retain the complete qualification and sanitized native receipt.

An intermediate broad run preceded complete dependency-link setup and used
NAS-backed runtime temporary state; existing helper and lock/concurrency controls
failed. That log is retained privately. Accepted qualification uses the matching
installed dependencies and local APFS temporary files with unchanged assertions;
no filesystem gate or test is suppressed.

## Remaining acceptance

Linux CI installs native Mosh and runs this case explicitly after real SSH
qualification. All pre-existing steps, assertions and deadlines remain intact.
Hosted exact-head results are still required. This native loopback fixture is
not a physical remote-host, real identity-provider, hardware-key, native-SSO,
client-certificate, Windows Mosh or assistive-technology acceptance claim.
Those broader #110 requirements and dependency audit #429 remain open. No
release, signing or provider lifecycle ownership change is made here.
