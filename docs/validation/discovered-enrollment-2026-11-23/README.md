# Discovery-driven authenticated peer migration

This is a native component result for [#631](https://github.com/jordanhubbard/webmux/issues/631). It connects actual mDNS observations to the production discovery API, authenticated enrollment and background peer maintenance. It does not complete #631 or MAC convergence.

At source `cb4e678`, a race-enabled native macOS ARM64 controller and a standalone `CGO_ENABLED=0` Linux ARM64 receiving API fixture pass:

1. Start a fresh owned Linux TLS listener and advertise its actual node ID/port through production node discovery.
2. Observe that exact node, address and port through the controller's administrator discovery API within eight seconds. Anonymous and foreign-account requests are refused. Discovery alone leaves the configured directory empty.
3. Explicitly configure the observed endpoint with a newly generated temporary owner grant, verify its node/account/TLS-key identity and fetch sessions through the normal federation route.
4. Close the original owned TLS listener and advertisement, then advertise the second actual listener with the same persistent identity. Observe the changed port through mDNS/API within eight seconds.
5. Let the unmodified ten-second production maintenance timer discover, prove and persist migration. The client does not supply a candidate or call reconciliation directly. Its twenty-second migration observation bound is separate from the unchanged production eight-second round/three-second proof bounds.
6. Fetch sessions at the migrated endpoint, reconstruct the controller against its retained private store, verify the same peer/account/key pin, and fetch again.

The client finishes in 11.12 seconds; the independent receiver finishes in 11.51 seconds and records **seven identity proofs and five bearer requests**, all on proven TLS connections. Both exit zero, the source inventory is unchanged, and the macOS race detector reports no race.

The earlier standalone positive run at `ab7145a` also passed. Its original source, binaries, logs and receipt remain retained; it is not relabeled as the later race run.

## Absence control and retained outcomes

The separate `--without-advertisement` run at the same `cb4e678` source creates a fresh API fixture and identity but makes no announcement. Its controller fails specifically at the eight-second discovery gate (8.07 seconds), before enrollment. The command exits one and its receipt remains `passed: false`. The remote controller then terminates only its owned server process, whose recorded exit is -15. This demonstrates refusal without an advertisement; it does not establish packet-loss, firewall or all multicast-isolation behavior.

An existing-output invocation refuses before any build or remote command and preserves the original receipt hash. The new runner and its embedded remote controller compile as Python. A preliminary syntax-check command used the server subdirectory for the root-relative script and failed with `FileNotFoundError`; rerunning from the repository root passes. That setup failure is separate from the native outcomes.

The complete local API race suite and vet pass at the same source. Windows ARM64 API test cross-compilation passes, without a native Windows execution claim.

The original explicit-candidate migration workload remains available without the new mode. Its assertions, direct-candidate scope and ninety-second receiver deadline are unchanged. No original failed multicast, listener-selection or other receipts are rewritten.

## Reproduce with owned resources

Use an already authorized native Linux ARM64 SSH target and verify its current non-loopback LAN bind address. The remote parent directory must exist. Both leaf output directories must be new and remain private:

```sh
python3 scripts/qualify-discovered-enrollment.py \
  --output /YOUR/PRIVATE/NEW/OUTPUT \
  --ssh-host YOUR_AUTHORIZED_HOST \
  --remote-root /YOUR/EXISTING/PRIVATE/PARENT/new-fixture \
  --remote-bind YOUR_VERIFIED_LINUX_LAN_IP \
  --race-primary
```

The runner builds both exact API test binaries, verifies the Linux executable before execution, checks native kernel/architecture, uses fresh random grants/identities, and binds the result to unchanged source inventories. Omit `--race-primary` for standalone binaries on both hosts. Use a separate fresh output namespace with `--without-advertisement` for the expected-failure control. Its command must fail; do not treat that as a passing positive run.

The [public receipt](receipt.json) records source/binary/log hashes and outcomes. Private raw output stays under `_build/discovered-enrollment631/`; it contains LAN metadata and disposable grant files. No installed account/sign-in store is copied. The certificate is trusted only by the fixture's explicit certificate pool; there is no TLS verification bypass or OS trust-store change.

## Limits

These are isolated production API/discovery components, not installed `cmd/webmux` services, browser enrollment, real coding agents or host reboot tests. The port move rebinds owned listeners/registrations while retaining the receiving API; controller reconstruction preserves the store but is not a process/OS reboot claim. Only macOS-controller to Linux-receiver enrollment/migration is qualified here. The receiver is not race-instrumented; cross-compilation does not qualify native Windows or Intel behavior. Full reverse/platform/IPv6 transport, certificate installation/renewal, duplicate/spoofed-node native controls, isolated networks, service expiry and the complete authorized ordinary/managed agent workflow remain open.

Local-only qualification follows the retained owner instruction: no new hosted jobs and `[skip ci]` on new commit/merge subjects. No installed runtime, service, network/trust/OS policy or shared-storage state changed.

## Reverse roles and polling-budget control

At source `c60273a`, the same workload also passes with a native Linux ARM64
controller and a race-enabled native macOS ARM64 receiver. It observes the real
advertisement, refuses anonymous/foreign discovery access and implicit enrollment,
enrolls the observed endpoint, follows its port change through the production
background timer, and retains the enrolled identity after controller
reconstruction. The receiver independently records seven proofs and five bearer
requests on proven TLS connections. Both processes exit zero. The updated runner
also passes the original macOS-controller/Linux-receiver direction at this source.
Neither direction changes the original observation bounds or injects candidates.

The first reverse positive at `3658bef` passed, but its silent-advertiser control
failed with HTTP 429 at 7.75 seconds. The fixture's 25ms polling can consume the
unchanged 300-request/minute API budget before the eight-second discovery bound.
That original failed control remains retained; it is not discovery-refusal proof.
The fixture now polls every 100ms, keeping both discovery windows within the
budget. HTTP-success assertions, actual discovery, production rate limits and
the eight-second bound are unchanged.

The corrected silent-advertiser control at `c60273a` exits one specifically because
the exact listener was not observed within eight seconds (8.14 seconds including
test cleanup). Its receipt remains `passed: false`, with no 429 response. The
controller terminates and waits for its owned macOS receiver, recording exit -15.
This is an absence control, not a general multicast-isolation qualification.

Use the same fresh-directory and authorized-host prerequisites above:

```sh
python3 scripts/qualify-discovered-enrollment.py \
  --output /YOUR/PRIVATE/NEW/OUTPUT \
  --ssh-host YOUR_AUTHORIZED_LINUX_ARM64_HOST \
  --remote-root /YOUR/EXISTING/PRIVATE/PARENT/new-fixture \
  --reverse --local-bind YOUR_VERIFIED_MACOS_LAN_IP --race-primary
```

`--race-primary` instruments the local macOS receiver in reverse mode. The remote
Linux controller remains a standalone `CGO_ENABLED=0` binary. The runner verifies
native architectures and executable digests, captures both witnesses and checks
unchanged source inventories. Existing-output refusal preserves the prior receipt
before building or contacting SSH. Both the local and embedded remote Python
controllers compile. Raw output remains private under `_build/discovered-enrollment631/`;
the [reverse-role receipt](reverse-receipt.json) records source/binary/log bindings
and preserves the original failed outcomes.

This adds reverse native API-component evidence. Installed applications, browser
enrollment, OS trust, native Windows/Intel/IPv6 coverage, real agents and the other
complete-workflow requirements remain open. It does not close #631 or #664.
