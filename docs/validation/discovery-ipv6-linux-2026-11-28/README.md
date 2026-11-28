# Native Linux discovery with explicit IPv6 application endpoints

Issue [#631](https://github.com/jordanhubbard/webmux/issues/631) remains open.
This change fixes discovery advertisements for explicitly bound Linux listeners
and extends the existing protected application qualifier to IPv6 global/ULA
addresses. IPv4 remains the default. Wildcard listeners retain interface-based
address selection; native macOS Bonjour remains OS-authoritative.

## Failure and cause

The first native IPv6 application run at source
`7d48b61710b88554f9aa1339861e022083a60d5c` failed the unchanged eight-second
exact-candidate gate. Both actual applications had started on their IPv6
listeners. Independent native IPv6 TCP duplex and exact-nonce UDP5353 multicast
measurements passed in both directions, without network/firewall changes.
Those carrier measurements are separate from application acceptance.

Pion mdns v2.2.2 service PTR/SRV handling asks for an IPv4 interface address first
and falls back to IPv6 only when no IPv4 address is selected. WebMux previously
passed the bound port, but omitted the listener address. On a dual-stack
interface this could announce an IPv4 endpoint for an IPv6-only application
listener. The retained failed receipt is included by hash in [receipt.json](receipt.json).

The application now propagates its actual bound address and uses Pion's
`WithLocalAddress` for a concrete Linux listener. Address admission preserves
wildcard behavior, normalizes mapped IPv4, and rejects unsafe or scoped
link-local explicit bindings. Unit controls cover these boundaries. The native
application gates still require the independently generated node ID, exact
listener address/port, and TLS; no assertion or deadline was weakened.

## Corrected native results

At integrated source `bff9e3d2843fa161c2833abbd41a3d24a26e764a` (including main
`f05959e4982845337ea4dd09f4b399632523e725`), both explicit IPv6 and
unchanged-default IPv4 application lifecycle commands pass all eight checks:

1. Exact HTTPS candidate discovery without automatic enrollment.
2. A direct native TLS connection whose remote endpoint matches the selected
   address family and receiver listener.
3. Browser-driven credential configuration and explicit node/account identity
   verification, followed by authorized session access.
4. Background port migration after receiver restart, with the browser closed.
5. Controller restart retaining the verified peer and migrated endpoint.
6. Receiver departure/expiry retaining the configured identity and reporting
   the peer unavailable.
7. Rejoin retaining identity without duplicate enrollment.
8. Policy restart disabling discovery while the verified peer stays accessible.

Separate fresh IPv4 and IPv6 changed-node controls both observe verification
status 502 and an unpinned directory. Their deliberate command exit code is 1,
`passed` remains false, and the specific refusal flag is required for admission.
The positive and negative controls within each source revision use the same
native executable digests. Earlier corrected runs at source
`064dd19e39272cf08906de0ee01b62e5e12db792` are separately retained.
All source inventories remain unchanged and every owned cleanup completes.
Independent exact-executable process scans find zero remaining fixture servers
on either native host.

Both images are built on Linux x86_64; the ARM64 image is cross-built, transferred
with digest verification, and actually executed on native Linux ARM64. Browser
control uses the existing Chrome through an owned loopback SSH forward to the
controller; the direct TLS witness measures the native peer endpoint. The
protected homes, credentials,
CA, logs, screenshots and LAN addresses remain private. Only process-local
fixture certificate trust is supplied to the Linux controller.

## Reproduction and remaining scope

Use the existing application qualifier from a committed clean checkout with a
fresh private output directory and authorized native Linux ARM64 SSH peer:

```sh
PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/PATH/TO/INSTALLED/CHROME \
  GOMAXPROCS=2 GOFLAGS=-p=2 node webmux/scripts/qualify-discovered-application.mts \
  --output /YOUR/PRIVATE/NEW/OUTPUT --peer AUTHORIZED_LINUX_ARM64_HOST \
  --local-bind RECEIVER_GLOBAL_OR_ULA_IPV6 --remote-bind CONTROLLER_GLOBAL_OR_ULA_IPV6 \
  --local-interface RECEIVER_INTERFACE --remote-interface CONTROLLER_INTERFACE \
  --ip-family ipv6 --lifecycle
```

Use a separate fresh output and `--changed-node-proof` for the negative control.
For IPv4, supply IPv4 addresses and omit `--ip-family` to exercise the default.
The original eight-second discovery and 185-second departure bounds remain.
SSH forwarding and remote ephemeral-port allocation now support the explicit
family, and initial candidate snapshots are retained privately on failure.

This is IPv6 application-endpoint qualification on a dual-stack multicast LAN.
It does not attribute application discovery packets to one multicast family or
qualify an IPv6-only network. Scoped link-local IPv6, isolated-network/spoofing
variants, other native platforms, installed-service upgrades, OS trust
provisioning, real-agent/MAC authority and the complete #664 workflow remain
open. No installed service, network policy, real account or canonical MAC
registration is changed.

Delivery retains the existing [local-only policy](../discovery-lifecycle-2026-11-23/README.md):
run repository gates locally and retain `[skip ci]` on source/evidence commits
and the merge subject. These receipts do not create a new hosted or whole-fleet
qualification claim.

## Repository gates

The integrated source passes the complete local `make test` and `make lint`
gates: 500 frontend tests, 302 helper tests (three existing skips), and
1418 E2E cases (22 existing skips), plus the maintained Go, race and
strict TypeScript checks. [local-gates.json](local-gates.json) binds the source
and complete private log hashes. Application source bytes are unchanged by the
subsequent evidence/roadmap documentation. The earlier pre-integration full job
was deliberately interrupted before its main browser matrix so the new main
worker-authority implementation could be included; it is not a passing gate.
