# Manual peers while discovery is unavailable

Issues [#631](https://github.com/jordanhubbard/webmux/issues/631) and
[#664](https://github.com/jordanhubbard/webmux/issues/664) retain their remaining
acceptance. This qualification adds a separate `--manual-peer-fallback` scenario
to the protected native application qualifier. It exercises a real discovery
initialization failure and the actual manual Add server interface.

## Native application evidence

The controller requests browsing on a freshly generated interface name that an
independent native `socket.if_nameindex()` probe verifies is absent. Advertising
is disabled on that controller. Its actual discovery API reports browsing
requested, `started: false`, no candidates, and the exact public error:

> Network discovery unavailable; manually configured peers remain available

An independent inspection of the owned controller process's socket inodes finds
no UDP or UDP6 sockets. Those API and socket checks run initially and again
after each controller and receiver restart. The receiver advertises normally;
no firewall, network interface or installed service configuration is changed.

The browser sees the error and uses Add server with an explicit HTTPS IP endpoint
(bracketed for IPv6), enters a disposable credential, and explicitly verifies the
actual node/account identity (HTTP 204). Authorized session inventory returns
200. Controller restart preserves the same one manually configured peer. Receiver
stop returns 502 while retaining its identity; restart restores 200 and the same
one entry while discovery remains unavailable. These five checks pass for both
IPv4 and IPv6 on native Linux x86_64 receiver / Linux ARM64 controller.

All six final cases use exact integrated source
`5b8d882bc5392aaae14093bb748ff54995a5aa44`, including main
`a0f803a1022c3df0f1fda2342d380daaa6a0bde7`, and identical native executable
hashes across the matrix:

| Scenario | IPv4 | IPv6 |
| --- | --- | --- |
| Unavailable discovery, manual configuration, both restarts | 5 checks pass | 5 checks pass |
| Original discovery/enrollment/migration/expiry/rejoin/disabled-policy lifecycle | 8 checks pass | 8 checks pass |
| Changed advertised identity before explicit verification | Expected refusal | Expected refusal |

The negative cases deliberately exit 1 with `passed: false`; admission additionally
requires the specific changed-node refusal flag, HTTP 502 and no identity pin.
An arbitrary command failure is not admitted. Direct native TLS diagnostics
independently verify the actual selected receiver endpoint/address family.
The original lifecycle candidate and departure bounds remain unchanged.
Earlier manual successes at source `8edac2e08c7aed8f6928802f665f7e7e2dd1ed9c`
are retained separately from the complete integrated matrix.

Every case retains an unchanged tracked-source inventory and completed owned
cleanup. Independent exact-executable process scans find zero remaining fixture
servers on either native host. [receipt.json](receipt.json) records source,
binary and private log/receipt hashes, sanitized observations and cleanup.
The ARM64 image is cross-built on x86_64 and actually executed on native ARM64;
this is not an emulated target claim. Browser control uses an owned loopback SSH
forward and installed Chrome. The protected profiles, credentials, grants, CA,
screenshots and LAN addresses remain private. Fixture trust is process-local.

The incompatible combinations with `--lifecycle` or `--changed-node-proof` both
refuse before output creation or SSH. Every original embedded Python payload is
byte-identical and all current payloads parse, guarding indentation when nested
JavaScript changes around raw Python strings.

## Reproduction and scope

Run from a clean committed checkout, with a fresh private output directory and
an authorized native Linux ARM64 SSH peer:

```sh
PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/PATH/TO/INSTALLED/CHROME \
  GOMAXPROCS=2 GOFLAGS=-p=2 node webmux/scripts/qualify-discovered-application.mts \
  --output /YOUR/PRIVATE/NEW/OUTPUT --peer AUTHORIZED_LINUX_ARM64_HOST \
  --local-bind RECEIVER_IP --remote-bind CONTROLLER_IP \
  --local-interface RECEIVER_INTERFACE --remote-interface CONTROLLER_INTERFACE \
  --manual-peer-fallback
```

For IPv6 supply global/ULA IPv6 endpoints and add `--ip-family ipv6`. Use separate
fresh output directories with `--lifecycle` and `--changed-node-proof` for the
original positive and negative controls. The generated absent controller
interface is selected internally only in manual fallback mode.

This is a real controller discovery initialization failure on an otherwise
working dual-stack LAN. It does not qualify network isolation, multicast packet
loss/filtering, IPv6-only multicast, scoped link-local addresses, multiple
interfaces, duplicate names, other native platforms, installed-service upgrades,
OS trust provisioning, real-agent/MAC authority or the complete #664 workflow.
Those requirements remain open. No authenticated model turn or production
restart is performed.

## Repository gates

The exact integrated source passes complete local `make test` and `make lint`:
500 frontend passes, 308 helper passes with three existing skips, and 1418 E2E
passes with 22 existing opt-in/platform skips, plus Go, race and strict TypeScript
checks. [local-gates.json](local-gates.json) binds the exact source and full private
log hashes. The test keyring and fallback tmux server are isolated; fixture-owned
`TMUX_TMPDIR` selection is preserved. Application assertions, deadlines and host
security policy are unchanged. Subsequent evidence/roadmap edits change no
application or qualifier bytes.

Delivery preserves the existing [local-only qualification policy](../discovery-lifecycle-2026-11-23/README.md):
repository gates run locally and `[skip ci]` remains on source/evidence commits
and the merge subject. These receipts do not claim hosted or whole-fleet
qualification.
