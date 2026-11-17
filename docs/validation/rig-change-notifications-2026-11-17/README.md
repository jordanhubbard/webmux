# Native Rig change-notification qualification

Candidate for #443 under #194; integration and release gates remain required.

The grid uses a local Rig change as a reason to re-read canonical inventory. It
never derives seat state from the event body. The browser-facing
`webmux-rig-changes/v1` response contains only its contract and two booleans,
`available` and `changed`. Unknown builds or unavailable streams keep the bounded
canonical polling and observation expiry already used by the grid.

The qualified daemon is OpenRig 0.6.14 (df1c1517). Its GET `/api/events?replay=0`
subscribes before querying the log; no Last-Event-ID is sent. WebMux does not
request historical events or forward raw payloads. The stream reader accepts
bounded sequence-framed JSON only as invalidation, not as lifecycle authority.
Explicit other endpoints and unqualified builds do not silently use the local
daemon. The configured local scope ignores an unrelated CLI host selection.

## Bounds and lifecycle

Only administrator-authorized workspaces containing Rig tiles subscribe. The
server also checks integration enablement and platform availability. One wait
per owner and eight waits per server bound child processes; saturation returns
unavailable rather than queueing. A wait holds no coordination mutation mutex.
Node CLI checks each have a two-second limit; the stream waits at most twenty
seconds and the enclosing request at most twenty-five. The body is capped at
256 KiB, each frame or unfinished frame at 64 KiB and each wait at 128 frames.
Redirects are refused. Responses cannot be cached.

Both the wait and its canonical inventory read use the existing 1200-per-minute
discovery budget. The general 300-per-minute budget remains independent. A quota
control fills discovery beyond the old general threshold, verifies configuration
remains usable, then reaches discovery rejection at its unchanged limit. The
initial regression run exposed HTTP 429 during settings restoration because the
new traffic originally shared the general bucket; its failure and diagnostic
response are retained, not counted as passing acceptance.

The service flushes a subscribed header only after the daemon accepts the live
stream. This lets the native test establish readiness without sleeping. The
result body follows the first valid event or bounded end of the wait. Completion,
request cancellation and malformed/oversized streams close their readers and
child processes. Cancellation also kills the wait's own private process group,
including a CLI child stuck in its readiness check; a native Node/sleep-child
control verifies this separately. Browser unmount/disable cancels its wait; unavailable streams
back off thirty seconds, and successful waits have a one-second reconnect gap.
Ten-second non-overlapping canonical fallback reads and thirty-second expiry
remain. An invalidation during a canonical read queues one additional read.
Events in reconnect gaps rely on that fallback; this is not a durable event feed.

## Actual native result

The [native receipt](native-notifications.json) records published Rig, tmux 3.8,
Node 24.21.0 and Chrome 158.0.8127.36 on macOS ARM64. An isolated real Rig project
has two builtin terminal seats. The first is opened through the production API
at grid position (1,1). After the actual daemon subscription is ready, browser
fallback timers are paused. An explicit owned fixture seat stop produces a
payload-free notification, canonical re-read and detached/unknown tile state.
The sibling PID, viewer identity and position stay unchanged. After closing the
page, two new native subscriptions can be opened and canceled, proving that the
owner slot and upstream connections are released. The native case passes with fallback timers paused and both later subscriptions
canceled. Per-run durations and source bindings are retained in the manifest.

A preceding native case also passed; the final case adds cancellation/reuse.
The exact same test against unchanged parent cd32c86 fails at the missing live
subscription after observing its ordinary running/ready tile. No runtime source
is overlaid. The [negative receipt](parent-negative.json) binds that test and
retained log; its test overlay is restored.
The fixture overlay from PR #433 disables tmux 3.8's newly enabled mouse default
on the private test server. It changes no application runtime and is restored
and SHA256-checked in a finally block. The ordinary installed coordinator and
user tmux server are not modified. This does not qualify authenticated coding
agents, remote-host notifications, arbitrary other Rig builds, provider
adoption, or an Agent Deck identity/event bridge.

## Checks and remaining gates

Stream controls cover every byte split, mixed LF/CRLF frames, no replay header,
private payload projection, malformed events, unsupported versions/endpoints,
size/frame caps, quiet timeout and reader cancellation. Go controls cover
administrator access, disabled-provider isolation, owner/global concurrency,
local host scope, native Node transport/projection, cancellation and mutation
lock independence. Frontend controls cover event-driven re-read, coalescing,
backoff, ordinary workspaces, hung requests, disable/unmount and stale expiry.

The [manifest](source-manifest.json) binds source and retained receipts. Required
hosted CI, integration review and dependency audit #429 remain gates. The parent
Rig stack (#432/#437/#440/#441), private fixture #433 and independent Agents CI
coverage #435 must be reconciled before final main qualification. This work does
not close the broader #194 or recovery #318 acceptance requirements.
