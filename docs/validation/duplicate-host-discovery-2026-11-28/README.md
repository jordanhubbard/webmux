# Native duplicate-host discovery qualification

Issue [#631](https://github.com/jordanhubbard/webmux/issues/631) remains open.
At source `02bb515b71007b9f956b39eae504b801d7eab9a3`, two real protected
Linux x86_64 WebMux receivers share their physical host's advertised hostname,
with distinct node IDs, independent TLS public keys, ports and private homes.
An independent native Linux ARM64 controller observes them over the same LAN.
Both IPv4 and IPv6 cases pass seven actual application/browser checks.

## Acceptance measured

- Discovery exposes two separate candidates for the shared hostname. Neither
  candidate automatically becomes a configured peer. Both protected receivers
  refuse unauthenticated session access with HTTP 401.
- Choosing the untrusted duplicate in the real management dialog prefills its
  exact URL while leaving the credential blank and the directory empty.
  Cancelling does not enroll it. The user-driven positive control then supplies
  only the primary's fixture grant and explicitly verifies its identity.
- Only the primary is configured. After the duplicate stops, all 88 recorded
  observations per family retain the live primary candidate, the same sole
  configured identity and successful authorized inventory access. The duplicate
  expires after 89.069 seconds (IPv4) and 89.237 seconds (IPv6).
- The existing primary port-migration and controller-restart checks also pass.
  The original 185-second departure bound, discovery bounds and assertions are
  unchanged. The new continuity gate retains every thrown assertion/transport
  failure so polling retries cannot conceal an outage; both final runs record
  zero continuity failures.

The original eight-check IPv4 lifecycle regression passes unchanged at the same
source. The paired IPv6 changed-node negative control observes verification 502
and no identity pin, then deliberately exits 1. Admission requires those exact
observations and completed cleanup; arbitrary failures cannot satisfy it.
All four final native cases use identical receiver/controller executable hashes.
Independent exact-executable process scans find no remaining owned fixture
servers on either host. Tracked sources remain unchanged in every native case.

Complete repository `make test` and `make lint` pass at that source: Go/race,
strict TypeScript, 500 frontend, 308 helpers (three existing skips) and 1,418 E2E
passes (22 existing skips). Their actual log hashes are in the [receipt](receipt.json), together with native image,
qualifier, source-inventory and private artifact identities. Existing integration
and platform skips retain their original requirements. Three incompatible-mode
controls refuse before output creation, and all nine original remote Python
payloads are byte-identical to the preceding main source.

## Reproduction

Use a clean committed checkout with dependencies prepared (`make deps`),
qualified Node/Go tools, an installed supported
Chromium, authorized SSH access to a native Linux ARM64 peer, actual LAN bind
addresses/interfaces and a fresh private output directory:

```sh
PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/PATH/TO/INSTALLED/CHROME \
  GOMAXPROCS=2 GOFLAGS=-p=2 node webmux/scripts/qualify-discovered-application.mts \
  --output /YOUR/PRIVATE/NEW/OUTPUT --peer AUTHORIZED_LINUX_ARM64_HOST \
  --local-bind RECEIVER_LAN_IP --remote-bind CONTROLLER_LAN_IP \
  --local-interface RECEIVER_INTERFACE --remote-interface CONTROLLER_INTERFACE \
  --duplicate-host
```

Use explicit IPv6 bind addresses and `--ip-family ipv6` for the IPv6 case.
The duplicate scenario is separate from `--lifecycle`, `--changed-node-proof`
and `--manual-peer-fallback`; each needs its own fresh output. Retain raw receipts,
source checks and completed cleanup. The two preliminary runs before continuity
review remain retained privately; the final acceptance above uses the tightened
source and fresh reruns.

## Boundaries

This is two WebMux receiver instances sharing one actual host, plus one controller
on a second physical host. It does not establish behavior for conflicting host
address records from different machines, arbitrary forged/stale DNS packets,
isolated multicast, multiple physical interfaces, IPv6-only networks or other
native platforms. The untrusted control proves candidate/directory separation,
empty credential prefill and unauthorized session refusal; it does not constitute
an audit of every possible credential-exfiltration path. Real-agent/MAC workflow
acceptance stays under #664 and its ordered prerequisites.

All homes, grants and certificates are disposable test fixtures; raw data stays
private. No installed service, OS trust store, network policy or real account
changed. Delivery follows the [existing local-only qualification policy](../discovery-lifecycle-2026-11-23/README.md):
use `[skip ci]` in evidence commits and the merge subject, run repository gates
locally, and do not request new hosted qualification jobs.
