# Native macOS Bonjour node advertisements

The macOS advertiser now registers node and actual legacy listener-port metadata
directly through the system Bonjour API. The source-bound production-component
qualification discovers the exact independently generated peer ID in both
macOS ARM64/Linux ARM64 directions and simultaneously. Each browser also finds
that peer's exact HTTP/HTTPS instance records and declared ports. Node discovery
and those exact records pass; the separate host-only selection control remains
failed under [#660](https://github.com/jordanhubbard/webmux/issues/660).

Issue [#631](https://github.com/jordanhubbard/webmux/issues/631) remains open.
These endpoints declare listener metadata without opening HTTP listeners.
No TLS handshake, authentication, enrollment, address migration, native agent
workflow or installed-service acceptance is established by these probes.

## Implementation and lifecycle

The adapter uses the public functions declared in Apple's `dns_sd.h`, dynamically
loaded from system `libSystem.B.dylib` through pinned
[PureGo v0.11.3](https://github.com/ebitengine/purego/tree/v0.11.3). The standalone
server still builds with `CGO_ENABLED=0`. Linux and Windows dependency graphs
exclude PureGo; their discovery transport remains Pion. Native macOS browsing
also remains Pion. No command process wraps `dns-sd` in production.

Bonjour owns the OS-authoritative hostname and address records. The raw `Host`
hint does not create a separate alias on macOS. The same server policy scopes
node and legacy service registrations; explicit validated interface names become
native indexes. The instance retains its stable ID, and `NoAutoRename` requests
conflict reporting. Startup requires all daemon acknowledgments, with a
three-second acknowledgment bound after requests are submitted. Runtime errors
are retained, cancel discovery, release owned registrations and appear as
unavailable through the administrator discovery API. Manual peers remain usable.

One process-lifetime callback routes opaque native references to their event-loop
owner. No Go data pointer is retained by the native API. TXT bytes cross the FFI
as a byte pointer, including Darwin ARM64 stack argument packing. Closing or
canceling a browser releases its transports exactly once. Deterministic race tests
cover scoped registration, partial startup failure, missing acknowledgment,
late daemon failure, terminal error retention and cancellation cleanup.

Actual native registration controls run with both `CGO_ENABLED=0` and the race
build. A real duplicate listener produces daemon error **-65548**. The daemon can
acknowledge a new request and subsequently reject the earlier registration; it
does not promise which request wins. The control requires the surfaced conflict,
cleanup of the losing reference/callback entry, successful close of the remaining
registration and successful reuse after owned cancellation. It does not claim
that an existing advertisement is protected from every duplicate-name conflict.
No provider or HTTP service is stopped by these registration controls.

## Native results

| Probe | macOS endpoint | Linux endpoint | Result |
| --- | --- | --- | --- |
| Linux advertises; macOS browses | Exit 0 | Exit 0 | Exact node and HTTP/HTTPS instance records pass |
| macOS advertises; Linux browses | Exit 0 | Exit 0 | Exact node and HTTP/HTTPS instance records pass |
| Both advertise and browse | Exit 0 | Exit 0 | Both exact peers and their listener records pass |
| Separate system Bonjour registration | Owned SIGINT, exit -2 | Exit 0 | Exact control peer found |

The exact-record qualification exits **0**. The system-command registration is a
separate control. It remains alive until Linux observes it, then the harness owns
its SIGINT cleanup. It is not substituted for the production adapter.

Candidate address records include IPv4, with IPv6 also observed by Linux in the
simultaneous phase. Receiving an AAAA record does not establish an IPv6 multicast
transport path or full dual-stack acceptance. The Linux binary is cross-built on
macOS, hash-verified after transfer and executed on native Linux ARM64.

Run from a macOS ARM64 checkout with already authorized SSH access and an existing
private remote parent. Both output leaf directories must be new:

```sh
python3 scripts/qualify-discovery-two-host.py \
  --output "$PWD/_build/bonjour-owned" \
  --ssh-host YOUR_AUTHORIZED_HOST \
  --remote-root /YOUR/EXISTING/PRIVATE/PARENT/bonjour-owned \
  --bonjour-control --legacy-instance-records
```

The original eight-second browse/both bounds and 12-second advertisement interval
remain. The driver rebuilds binaries, verifies transferred hashes and re-enumerates
sources before and after execution, so added/removed files also invalidate the
receipt. Source-inventory controls demonstrate both failure cases in a private
synthetic tree. Existing output is refused. Output contains private LAN addresses;
keep it private. The [public receipt](receipt.json) publishes outcomes and hashes.

The same retained `CGO_ENABLED=0` Darwin test binary also executes the actual
registration conflict/cancellation control, selected with
`WEBMUX_TEST_BONJOUR_INTERFACE=<owned-interface>` and
`-test.run='^TestNativeBonjourRegistrationControls$'`. Without explicit native
fixture variables, these network controls skip.

## Retained failures and remaining work

Adding `--legacy-listeners` also requires the existing host-only `Resolve` path to
select the fixture's ports. That qualification exits **1**: all exact nodes and
instance records pass, but Linux selects HTTPS **8443** instead of fixture **19311**
on the Mac's shared hostname. An installed WebMux listens on 8443. The forward
Linux-hosted fixture resolves HTTP 19310 and HTTPS 19311. #660 tracks ambiguity;
no assertion is changed to accept the wrong port.

The unchanged same-host legacy tests still exit 1. The raw legacy advertiser
lookup returns default 8443 instead of 19443. The node test now observes its node,
but its synthetic hostname alias is not registered by native Bonjour, so its
legacy lookup also fails. Their assertions/deadlines remain unchanged; they do
not replace real-host exact-instance evidence or establish client compatibility.

Earlier failures remain retained: initial direct-API prototypes used a nonexistent
library alias and then an incorrect slice stack argument; the exact-instance
fixture initially compared trailing-dot DNS names inconsistently; the first
collision control assumed the newer registration must lose. Direct API traces in
both build modes established the later-conflict behavior, and the corrected
control verifies either reported loser with strict cleanup. Initial broad Go
testing used system Node 26, which the native Rig path explicitly refuses; the
same test and complete Go suite pass with the qualified Node 24 runtime.

Full Go tests with Node 24, focused discovery/API race tests, vet, the production
server contract and standalone build pass. Darwin AMD64 and Windows
ARM64 have compilation-only scope; Linux ARM64 also has the native component
execution described above. Required hosted CI and
packaging remain delivery gates. Complete discovered/enrolled native lifecycle,
platform, expiry, interface/address-change, spoofing and agent-workflow acceptance
remain open under #631/#630; this checkpoint checks no roadmap completion box.
