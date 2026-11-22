# Host-only listener ambiguity qualification

Component evidence for [#660](https://github.com/jordanhubbard/webmux/issues/660),
built on the [native Bonjour checkpoint](../discovery-bonjour-2026-11-22/README.md).
The [receipt](receipt.json) binds both native runs to the same discovery sources
and their exact CGO-disabled Darwin/Linux ARM64 test binaries.

## Behavior

Host-only `.local` lookup observes the existing three-second DNS-SD window before
selecting a listener. Distinct service instances on different preferred-protocol
ports return `ErrAmbiguousListener` and require an explicit URL with a port.
Equal-port duplicates remain usable, and updates replace the same instance.
HTTP still requires explicit permission and keeps its existing preference.
No-record legacy defaults remain 8443/8080. Parent cancellation returns the
cancellation error. Explicit URL/port paths bypass this lookup as before.

Address-refresh callbacks update one locked, bounded per-instance map directly.
There is no lossy event queue. The map holds at most 256 instances across both
services; exceeding that bound refuses selection instead of choosing from a
partial inventory. The limit remains exceeded for the remainder of that lookup.
This is observed ambiguity handling, not authentication: an untrusted or absent
advertisement cannot establish a server's identity or guarantee uniqueness.

## Native controls

Two independent native runs cover HTTPS and explicitly permitted HTTP. Each
passes forward, reverse and simultaneous production node discovery, followed by
an independent system `dns-sd` advertisement control. Exact expected peer IDs
and per-instance HTTP 19310/HTTPS 19311 records are required in every applicable
phase. Linux uses a fresh synthetic hostname so macOS must resolve its exact
HTTP and HTTPS ports. The Mac owns two separate native advertisements on its
system hostname, with ports 19310/19311 and 19309/19312; Linux must refuse the
selected protocol with the typed ambiguity error and an empty URL.

No installed server, network/trust policy or fixture assertion was changed.
Fixtures open no HTTP listeners. Original eight-second browse/both,
12-second advertisement and 20-second test bounds remain. Advertisements are
closed by their owning fixture; the separate system control is stopped by its
own driver. Observed candidate address families include IPv4 and IPv6 in the
simultaneous phase; this does not prove end-to-end IPv6 transport.

From native macOS ARM64, use new private output leaves and an already authorized
native Linux ARM64 SSH target with an existing private parent:

```sh
python3 scripts/qualify-discovery-two-host.py \
  --output "$PWD/_build/listener-https" \
  --ssh-host YOUR_AUTHORIZED_HOST \
  --remote-root /YOUR/EXISTING/PRIVATE/PARENT/listener-https \
  --host-selection-controls --ambiguity-protocol https --bonjour-control
```

Repeat with fresh output paths and `--ambiguity-protocol http`. This mode cannot
be combined with `--legacy-listeners`: the original exact-port compatibility
assertion on a shared hostname remains strict and is expected to fail when the
hostname has multiple distinct listener ports. Do not reinterpret that failure
as a successful exact-port lookup. Native logs contain private network details
and remain local; the public receipt omits addresses and session identifiers.

## Retained failures and limits

The original first-answer implementation chose installed port 8443 rather than
fixture port 19311. An intermediate bounded event queue correctly refused an
incomplete inventory but overflowed under real multi-address notifications and
failed the typed-ambiguity gate. Both failed attempts remain retained. The direct
bounded accumulator passes that gate; no timeout was extended or assertion
weakened. Deterministic unit tests cover competing ports, protocol preference,
equal ports, foreign/invalid records, updates, cancellation, the instance limit,
and 80,000 concurrent duplicate refreshes retaining a conflicting listener.

This component result does not complete #631 discovery/enrollment, address-change,
expiry, spoofing, native platform, agent-workflow or release qualification.
MAC convergence remains tracked by #664; discovery does not replace project
roadmaps or the shared worker-pool authority.

Focused discovery race tests, discovery vet and the production Go API/session
contract pass. The complete Go suite and hosted checks are separate delivery
gates; native component passes do not substitute for those gates.
