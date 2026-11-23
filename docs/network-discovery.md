# Network node discovery

Implementation checkpoint for [#631](https://github.com/jordanhubbard/webmux/issues/631).
Full native discovery and agent-workflow acceptance remain open. Authenticated
HTTPS identity enrollment and address reconciliation are implemented.

WebMux browses `_webmux-node._tcp.local` in the server process, independently of
open browser tabs or optional agent providers. Eligible network listeners also
advertise this service. Explicit `--listen` overrides and loopback-only listeners
remain private. Manage servers shows nearby candidates and can prefill the normal
connection form. A candidate does not become a connected server until the operator
configures the existing authenticated outgoing entry and receiving owner grant.

## Identity and authorization

The private WebMux home gets `config/node-discovery.yaml` on first eligible
startup. Its random 128-bit public ID persists across restarts. Use a separate
home for each server; copying a home also copies its identity. The DNS-SD instance
is `WebMux-<id>`. TXT contains exactly `v=1`, `id=<id>` and `tls=0|1`.
The SRV record identifies the selected HTTP or HTTPS listener's actual port.
Advertisements contain no account, project, session, credential or agent state.

An advertised ID is an **unverified claim**, not proof of identity. Raw candidates
do not authorize session access or change a saved peer. Administrators can use
**Manage servers → Verify identity** on an already configured HTTPS peer. The
receiving server proves possession of that peer's existing owner-grant credential
on a fresh, TLS-bound challenge. The controller pins the persistent node ID,
receiving account and TLS public-key SHA-256 in `servers.yaml`. It refuses a second
entry for the same verified node/account; separate accounts remain distinct.

The proof request contains a domain-separated HMAC selector and challenge proof,
never the bearer credential. The response HMAC binds a fresh nonce, node/account
identity and TLS exporter material. Every subsequent pinned HTTP or WebSocket
connection repeats the proof on the **same TLS connection** before sending a
bearer credential. A separate preflight connection would leave a verification/use
gap. Relayed responses from another TLS connection, replayed challenges, changed
nodes, accounts or TLS keys fail closed. Requests also name the original node and
account at admission, so reassignment between proof and use cannot expose a
foreign account. The proof grants no additional configuration/coordinator access.

With network browsing enabled, the server reconciles enrolled HTTPS peers in the
background. It prefers a reachable existing endpoint. When that endpoint is lost,
it can try matching candidates and persist a new origin/address only after proof
of the original node, TLS key and receiving account. No bearer credential is sent
while evaluating candidates. Interface policy scopes DNS-SD browsing and
advertising; a routed candidate still has to prove the original enrolled
identity. The original peer/session namespace remains intact;
multiple interfaces do not create extra peers or providers. Concurrent edits,
removals and credential changes refuse the stale update. The loop runs without
an open browser, checks at most four peers per ten-second round, limits a round
to eight seconds, limits each dial/proof to three seconds and tries at most four
candidates per peer. Both peer and candidate windows rotate to avoid a fixed
unavailable/spoofed prefix starving the directory.

HTTPS still validates certificates against the OS trust store and the advertised
hostname, including when dialing a discovered address directly. There is no TLS
verification bypass. Enrollment/reconciliation requires direct end-to-end HTTPS;
a TLS-terminating proxy's different exporter context is refused. TLS passthrough
can retain the end-to-end channel. Certificate renewals using the same key retain
the pin; key replacement or deliberate account/node reassignment requires removal
and explicit enrollment again. Ordinary connection edits preserve the pin.
Manual HTTP peers remain supported as before, but cannot enroll a verified node
identity or migrate automatically. These boundaries are intentional; discovery
must not turn an unauthenticated advertisement into credential forwarding.

## Server policy

Edit `config/node-discovery.yaml`, preserving the generated ID, then restart
WebMux through its normal service manager:

```yaml
version: 1
id: <preserve the generated 32-character lowercase hexadecimal ID>
browse: true
advertise: true
interfaces: []
```

`browse` and `advertise` are independent server-wide controls. `advertise: false`
also disables legacy listener-port advertisements from this service. An empty
interface list uses suitable multicast interfaces; a list such as `[en0]`
restricts both browsing and advertising. A missing, down or non-multicast explicit
interface refuses discovery rather than falling back to all interfaces. Invalid
configuration is preserved and reported; it never generates a replacement ID.

Multicast/socket failures do not prevent HTTP startup or manually configured
peer connections. Discovery requires multicast UDP 5353 on the selected network.
Manual peers remain the fallback on isolated networks. Discovery does not bypass
HTTPS certificate verification, and choosing an HTTP candidate does not check the
connection form's explicit HTTP permission on the operator's behalf.

On macOS, the server registers node and legacy listener services directly with
the system Bonjour API. Bonjour owns the current hostname/address records and
interface changes; the raw `Host` hint is not a separately registered alias.
Explicit interface policy becomes native interface indexes. Stable instance names
are not automatically renamed on collision. Startup waits up to three seconds
for registration acknowledgments after submitting the requests; rejection or a
later daemon/socket failure reports discovery unavailable. Cancellation releases
only this server's registrations. The standalone build remains `CGO_ENABLED=0`
using pinned PureGo bindings; no `dns-sd` helper process is launched. Browsing
continues through Pion. Linux and Windows retain their existing transport.

## Host-only listener selection

A bare `.local` host observes listener advertisements for up to three seconds.
If multiple instances advertise different ports for the preferred protocol,
lookup refuses the ambiguity and requests an explicit URL with a port. Equal-port
duplicates remain usable; updates replace the same instance. HTTP still requires
explicit permission. A bounded inventory overflow also refuses selection.
Explicit URL/port inputs bypass this lookup. These untrusted observations never
authorize a node. The [native ambiguity controls](validation/discovery-ambiguity-2026-11-22/README.md)
verify both HTTPS and HTTP refusal separately from exact per-instance discovery.

## Expiry and bounds

The current Pion dependency reports additions/changes, deduplicates identical
announcements, and has no departure callback. WebMux starts a fresh browse round
every 30 seconds. Candidate records expire 90 seconds after their last complete
observation; expiry is applied when receiving an observation or reading a
snapshot. Departure therefore has a bounded delay rather than immediate goodbye
handling. Manage servers rereads candidates every 15 seconds. The registry holds
at most 256 address/instance records, rejects invalid protocol metadata, and
preserves scoped IPv6 link-local addresses. It does not assert complete network
inventory or protection against advertisement flooding/spoofing.

## Validation and remaining acceptance

The [native Bonjour adapter qualification](validation/discovery-bonjour-2026-11-22/README.md)
adds production-component discovery in both directions and simultaneously between
macOS ARM64 and Linux ARM64. The browsers also find each fresh peer's exact legacy
HTTP/HTTPS instance records. Actual local registration conflict and cancellation
controls pass. This does not complete authenticated discovery/enrollment or the
remaining platform and lifecycle gates below. The integrated
[listener-ambiguity controls](validation/discovery-ambiguity-2026-11-22/integration-2026-11-23.json)
now verify typed refusal for competing host-only listeners under both protocol
policies; #660 is closed. Original failed receipts retain their outcomes. Exact
node candidates keep their own instance, ID and port.

The [discovery-driven enrollment qualification](validation/discovered-enrollment-2026-11-23/README.md)
connects actual native mDNS/API observations to authenticated enrollment and the
production background migration timer on macOS ARM64/Linux ARM64. A race-enabled
controller retains the enrolled peer/account/key identity after listener movement
and controller reconstruction; the independent receiver witnesses same-connection
proof before every bearer use. A fresh silent-advertiser control fails at the
discovery gate. These are isolated API components with explicit fixture trust,
not installed-service, OS trust-store, browser or real-agent
acceptance. Full #631 remains open.
The [reverse-role follow-up](validation/discovered-enrollment-2026-11-23/README.md#reverse-roles-and-polling-budget-control)
also qualifies the Linux ARM64 controller/macOS ARM64 receiver direction, with
an independent proof/bearer witness and an explicit silent-advertiser refusal.
Other platform and complete application/workflow acceptance remain open.

Deterministic tests cover malformed/foreign/self announcements, duplicate TXT
keys, unrelated service/domain, credential metadata refusal, invalid endpoints,
unscoped IPv6, address multiplicity, expiry/refresh, concurrent bounded snapshots,
stable policy/identity, interface refusal and administrator-only API access.
Dialog tests verify that discovery prefill neither enrolls nor supplies a
credential, that HTTP permission remains explicit, and that manual management
survives discovery failure.

On 2026-11-21 this Mac's native multicast attempt failed both the unchanged
`TestMulticastAdvertisementRoundTrip` and the new `TestNodeMulticastRoundTrip`.
The former returned its default 8443 rather than advertised 19443; the latter
received no candidate within eight seconds. A diagnostic rerun also failed;
private output is retained in `_build/discovery631/native-debug.log`. No test
assertion, deadline or multicast requirement was relaxed. This is failed local
network evidence, not a native success or an established cause. The same-host
fixture would not establish two-host acceptance even if it passed.

The [2026-11-22 transport diagnosis](validation/discovery-transport-2026-11-22/README.md)
reproduces both failures and adds a bounded, opt-in control using the production
socket setup. This Mac receives IPv4 loopback traffic but times out receiving
its own multicast questions on both physical interfaces and both address
families, despite successful joins/sends. The same Linux binary's physical
interface controls pass on IPv4 and IPv6; IPv6 loopback has a separate send
failure. The complete commands' failed outcomes remain retained. This narrows
investigation below candidate parsing without establishing a cause or changing
network policy. No product binding patch was justified.

A cross-compiled source-bound Linux ARM64 test binary passed on the existing
Sparky worker, including the original listener advertisement and the new node
multicast round trip (no multicast skips). This is a single Linux host with two
isolated endpoints, not cross-host enrollment or Mac acceptance. Its output is
retained privately in `_build/discovery631/linux-native-final.log`; the transferred
binary occupies about 6.3 MiB under the worker's private qualification directory.
No installed WebMux service or active agent was restarted.

The complete frontend suite passes (44 files, 476 tests), as do strict TypeScript
checks, frontend lint/build, discovery/API race controls and targeted Go vet.
An ambient Node 26 full Go run failed the existing native-create owner-directory
fixture. A Node 24 run then failed the existing Rig child-start cancellation
fixture; both pass individually with Node 24. A subsequent clean `go test -count=1 ./...` with Node 24 passes all packages.
Original full-run failures remain in `_build/discovery631/go-test*.log`; the
clean run does not establish the original child-start failure cause. Follow-up
[#633](https://github.com/jordanhubbard/webmux/issues/633) tracks that failure.

The native identity component additionally passes between a macOS ARM64 client
and a Linux ARM64 receiver over a routed connection, using isolated real WebMux
API instances, an ephemeral
fixture trust root and a random temporary grant. It proves enrollment, HTTP use
on the same TLS connection, original endpoint loss, authenticated address/port
migration and controller reconstruction with unchanged peer/account identity.
The receiver records seven proofs and five bearer requests, all on proven TLS
connections. The fixture grant is removed afterward. This does not establish OS
trust-store installation, native WebSocket networking on both platforms, mDNS on
the Mac, installed-package/host reboot, or real coding-agent coordination.
The opt-in harness is `node_identity_network_test.go`; private raw logs and exact
binary digests are retained under `_build/node-identity631/`.

Before closing #631, qualify the complete discovered/enrolled two-host workflow,
supported Windows/Linux/macOS boundaries, policy/interface changes, isolated
multicast, departure and address changes, duplicate/spoofed announcements and
background reconciliation with actual discovery events. Automatic session
participation and agent-aware delivery are separate unfinished requirements
under #630.
