# Native Linux x86_64 / ARM64 discovered application qualification

Issue [#631](https://github.com/jordanhubbard/webmux/issues/631) remains open.
The maintained application qualifier now accepts a Linux x86_64 primary receiver
as well as its existing macOS ARM64 primary. The independent peer controller
remains native Linux ARM64. Image filenames/hashes identify receiver and
controller roles, and the receipt records each actual platform and architecture.

At source `154139c86f3d3c7933c7804f3cfec1916dc8834c`, two real Linux hosts on the
same LAN pass all seven existing application/lifecycle checks. Playwright drives
the actual protected application UI:

1. The protected controller discovers the exact HTTPS receiver without enrolling it.
2. The browser-driven workflow saves the discovered URL/credential, explicitly verifies
   node/account identity and accesses the authorized session inventory.
3. Receiver restart with a changed port migrates in the background with the browser
   closed, retaining the enrolled identity.
4. Controller restart retains the same verified peer and migrated endpoint.
5. The stopped receiver expires from discovery while its configured identity remains
   retained and inventory reports unavailable.
6. The same receiver rejoins with its original identity and one configured entry.
7. A normal policy restart disables discovery and owned UDP sockets while the
   verified configured peer remains accessible.

A separate fresh changed-node control returns verification status 502 and leaves
identity unpinned. Its command deliberately exits 1 after observing that required
refusal; unrelated failures cannot satisfy the negative admission.

Both images were built on Linux x86_64. The ARM64 image was cross-built and hash
verified on transfer before actual execution on the independent ARM64 machine.
Both source inventories remain unchanged. All original observation and cleanup
bounds remain, including the eight-second initial discovery gate and 185-second
departure allowance. Positive and negative cleanup complete; independent process
checks find no remaining owned receiver/controller images.

The earlier unchanged production-component run at
`175ae6fd831fbffc2df4b18feaacf43c3e8bef82` additionally passes both directions,
simultaneous native discovery, exact per-instance HTTP/HTTPS record checks and
absent-peer refusal on both hosts. `component-receipt.json` preserves that separate
scope and its initial launcher-directory setup failure, which occurred before
network startup. The corrected launcher retained the same source and bounds.

Reproduce the application run from a clean committed checkout on a supported
primary, with the qualified Node/Go tools on PATH and authorized SSH access to the
Linux ARM64 peer. Use fresh private output directories and actual LAN interfaces:

```sh
PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/PATH/TO/INSTALLED/CHROME \
  GOMAXPROCS=2 GOFLAGS=-p=2 node webmux/scripts/qualify-discovered-application.mts \
  --output /YOUR/PRIVATE/NEW/OUTPUT --peer AUTHORIZED_LINUX_ARM64_HOST \
  --local-bind RECEIVER_LAN_IP --remote-bind CONTROLLER_LAN_IP \
  --local-interface RECEIVER_INTERFACE --remote-interface CONTROLLER_INTERFACE \
  --lifecycle
```

Run `--changed-node-proof` instead of `--lifecycle` with a second fresh output
directory for the required negative control. Inspect its explicit refusal flag,
verification status, unchanged sources and completed cleanup, not exit code alone.

`receipt.json` binds runtime images, maintained qualifier, tracked source
inventory and private artifact hashes. Raw LAN records, protected profiles,
fixture credentials, logs and screenshots remain private. Trust is process-local
Linux fixture trust; no installed service, OS trust store, network policy or
native user account changed. This run does not requalify macOS/Windows execution,
IPv6 transport, isolated multicast, every duplicate/interface/spoofing variant,
or the complete real-agent/MAC workflow under #664. Those acceptance gates remain
open.

Delivery retains this qualifier's [local-only policy](../discovery-lifecycle-2026-11-23/README.md):
use `[skip ci]` on the final evidence commit and merge subject, run the maintained
repository gates locally, and do not request new hosted qualification jobs.
Historical hosted results keep their original source; no new whole-current-source
hosted/platform result is implied by these native fixture receipts.
