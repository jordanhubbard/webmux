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
  restart it normally. Require browsing stopped, no candidates and continuing
  inventory access through the retained configured peer.

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
