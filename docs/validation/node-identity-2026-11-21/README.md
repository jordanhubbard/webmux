# Authenticated node identity and address migration

This is a component result for roadmap #631/#630, not completed discovery or
agent coordination. The implementation is described in
[network discovery](../../network-discovery.md).

A real TLS listener on the existing Linux ARM64 worker over a routed connection
and a real macOS ARM64
controller API instance pass the opt-in native protocol workload. The controller
explicitly trusts the isolated fixture certificate; this does not change or
qualify either OS trust store. A random, private, temporary owner grant is used
and removed afterward. The receiver opens only its isolated ephemeral fixture
listeners; no installed WebMux service or existing agent is restarted.

The workload performs HTTPS grant proof and durable node/account/TLS-key pinning,
reads through the real pinned federation proxy, closes the original receiving
listener, verifies the original identity at a new discovered-format origin/port,
keeps the same configured peer/account namespace, reads again, and reconstructs
the controller using its retained store. Candidate input is supplied explicitly:
this is not a native mDNS discovery result or a background-timer acceptance claim.

The receiver witnesses successful identity responses and subsequent bearer
requests using the exact accepted `net.Conn` identity. Its counters include seven
successful proofs and five bearer requests, with no bearer on an unproven
connection. Endpoint/CA metadata stays in private evidence. The published receipt
contains binary/source digests and redacted protocol output, not the grant.

Deterministic/race tests additionally refuse relayed responses from a different
TLS exporter, replayed challenges, altered node IDs, keys, account/credential or
trust changes, stale configuration writes, removed peers, duplicate node/account
pins, foreign owners, malformed candidate addresses and HTTP downgrade. HTTP and
WebSocket controls verify that the proof and credential use the same connection.
A grant reassigned after a valid proof is refused at application admission.
Directory edits preserve pins, distinct account scopes remain separate, and
manual HTTP peers retain their original supported behavior.

The clean full Go suite, Go vet, public server contract, strict TypeScript,
frontend lint/build, 476 frontend tests, and focused federation/identity race
controls pass. Windows ARM64 test cross-compilation is a compile result only.

Still required: native macOS mDNS recovery, Windows node networking, the complete
discovered two-host enrollment/migration workflow and background lifecycle under
real discovery events, installation/certificate trust and package qualification,
and the independent ordinary/managed real-agent project/claim/handoff/reservation
workflow under #630. Existing multicast failures and #633's historical fixture
failure remain recorded. Neither issue is closed by this component result.

An intermediate on-link-prefix guard rejected the routed two-host migration
fixture: the route used a VPN interface without an on-link IPv4 prefix. Its
client failed migration and its receiver reached the original 90-second timeout;
both failed outcomes remain retained. The guard would have narrowed reconciliation
for enrolled routed peers and was removed. DNS-SD interface policy still scopes
browse/advertise traffic; bounded candidate probes require the original node,
account, TLS key and channel proof before credential use. The failed experiment
and its source copies remain under `_build/node-identity631/`. This correction
does not establish LAN discovery or excuse the failed intermediate run.
