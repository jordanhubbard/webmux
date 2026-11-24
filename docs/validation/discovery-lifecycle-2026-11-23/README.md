# Native discovered-peer lifecycle qualification

Extends the actual protected application/browser qualifier for [#631](https://github.com/jordanhubbard/webmux/issues/631).
Run the existing documented command with `--lifecycle` in a new private output
directory. The original initial-discovery, browser enrollment, changed-port
migration and controller-restart checks remain required.

Additional phases require:

- Observe the receiver again after controller restart, before testing departure.
- Stop only the owned receiver. Require its candidates to disappear while the
  discovery browser remains healthy, its verified configured peer remains
  unchanged, and its scoped session inventory returns 502.
- Restart the same receiver home, key, node ID and port. Require rediscovery,
  inventory access and exactly the same single configured peer.
- Stop only the owned controller, set its fixture browsing policy to false and
  restart it normally. Require both browsing and advertising policies disabled,
  no candidates, no UDP sockets owned by that Linux controller process and
  continuing inventory access through the retained configured peer. The API's
  `started` field describes an initialized discovery object, including one with
  both policies disabled; it does not prove active browsing.

The post-restart and rejoin observation bounds are 38 seconds (one 30-second
browse refresh plus eight seconds of observation allowance). Departure has a
185-second observation bound: two 90-second candidate lifetimes plus five
seconds allowance, accounting for a cached DNS-SD answer refreshing the
application candidate lifetime. These are finite qualification bounds, not new
product promises. Every departure sample retains its wall-clock observation,
candidate last-seen and expiry times in the private receipt. Existing operation
deadlines and production behavior are unchanged. An absence caused by a failed
browser cannot pass the departure assertion.

The receipt binds the committed source, executable hashes, outcomes and owned
process cleanup. Passwords, grants, raw LAN records, fixture homes, logs and
screenshots stay private. This does not complete platform/IPv6, isolated
multicast, spoofing, installed-service or real-agent/MAC acceptance. #631 and
#664 remain open. Retain the local-only `[skip ci]` policy for this qualifier.

## Retained native result

At committed source `fe9c28d`, the macOS ARM64 receiver/Linux ARM64 controller
run exited zero with all seven original and lifecycle checks passing. The
receiver expired after 119.440 seconds across 119 retained snapshots, rejoined
1.559 seconds later without duplicate enrollment, and remained accessible after
the controller restarted with both discovery policies disabled and no owned UDP
sockets. All three cleanup operations completed and the complete tracked source
inventory stayed unchanged. All five strict TypeScript projects passed before
the native run; the tools project passed again after the final qualifier change.

Use installed Chrome explicitly when bundled Playwright Chromium is absent:

```sh
PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' \
  GOMAXPROCS=2 GOFLAGS=-p=2 node webmux/scripts/qualify-discovered-application.mts \
  --output /YOUR/PRIVATE/NEW/OUTPUT --peer YOUR_AUTHORIZED_SSH_HOST \
  --local-bind YOUR_MACOS_LAN_IP --remote-bind YOUR_LINUX_LAN_IP \
  --local-interface YOUR_MACOS_INTERFACE --remote-interface YOUR_LINUX_INTERFACE \
  --lifecycle
```

The [sanitized receipt](receipt.json) binds binaries, private artifacts and
outcomes to source. Complete private evidence is retained under the primary
checkout's `_build/discovery-lifecycle631/fe9c28d/`.

Two failed runs at `218d4c1` are retained separately. The first discovered the
receiver but could not launch the absent bundled browser. The second passed
departure and rejoin, then failed an incorrect qualifier assertion about the
API's `started` field. The final qualifier checks the disabled policies and
actual owned UDP sockets. Neither failed run is reported as a positive result;
both completed cleanup and retained unchanged sources. No production code or
deadline changed.
