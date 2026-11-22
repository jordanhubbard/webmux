# Network node discovery

Implementation checkpoint for [#631](https://github.com/jordanhubbard/webmux/issues/631).
Native network acceptance and authenticated identity reconciliation remain open.

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

An advertised ID is an **unverified claim**, not proof of identity. Candidates
never change a saved peer's address, deduplicate configured peers, initiate HTTP
requests, forward credentials, or authorize session access. Multiple addresses
and duplicate display names stay separate until an authenticated identity
exchange can reconcile them. That exchange, durable peer identity pins and
address migration remain required work under #631. The existing owner-scoped
federation grant is still necessary; it does not authorize coordination APIs.

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

A cross-compiled source-bound Linux ARM64 test binary passed on the existing
Sparky worker, including the original listener advertisement and the new node
multicast round trip (no multicast skips). This is a single Linux host with two
isolated endpoints, not cross-host enrollment or Mac acceptance. Its output is
retained privately in `_build/discovery631/linux-native-final.log`; the transferred
binary occupies about 6.3 MiB under the worker's private qualification directory.
No installed WebMux service or active agent was restarted.

The complete frontend suite passes (44 files, 474 tests), as do strict TypeScript
checks, frontend lint/build, discovery/API race controls and targeted Go vet.
An ambient Node 26 full Go run failed the existing native-create owner-directory
fixture. A Node 24 run then failed the existing Rig child-start cancellation
fixture; both pass individually with Node 24. A subsequent clean `go test -count=1 ./...` with Node 24 passes all packages.
Original full-run failures remain in `_build/discovery631/go-test*.log`; the
clean run does not establish the original child-start failure cause. Follow-up
[#633](https://github.com/jordanhubbard/webmux/issues/633) tracks that failure.

Before closing #631, qualify two real network hosts, supported Windows/Linux/macOS
boundaries, policy/interface changes, isolated multicast, departure and address
changes, duplicate/spoofed announcements, authenticated identity enrollment and
reconciliation. Automatic session participation and agent-aware delivery are
separate unfinished requirements under #630.
