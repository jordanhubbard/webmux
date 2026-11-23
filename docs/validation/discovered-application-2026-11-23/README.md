# Discovery enrollment through actual applications

References: [#631](https://github.com/jordanhubbard/webmux/issues/631),
[#701](https://github.com/jordanhubbard/webmux/issues/701).

This extends the earlier API-component qualification to two standalone
`cmd/webmux` executables, their normal startup/configuration paths and the actual
Manage servers browser interface. It does not complete #631 or MAC integration.

## Defect and correction

At `61b5418`, the Linux ARM64 controller discovered the macOS ARM64 receiver,
displayed its exact HTTPS candidate, and saved the browser-configured peer and
temporary credential. Verify identity failed. At `3d7a2e1`, an independent native
Go diagnostic using the same process-local certificate trust established that
direct-address TLS succeeds with the original hostname as `ServerName`, while
the controller's DNS resolver cannot resolve the advertised `.local` hostname.
The original failed application receipts and screenshots remain retained.

Initial enrollment previously relied only on ordinary DNS. It now tries the
configured endpoint first and may then try up to four fresh DNS-SD address hints
within an eight-second operation bound. Hints must match the configured HTTPS
hostname and port and pass address, expiry and advertised-ID validation. They
never replace the configured hostname or grant identity. Certificate trust and
hostname checks remain enabled; the existing TLS-bound grant proof runs on the
same connection. The proven node must match the hint's claim before its address
and pin can be committed. Existing pin/account/key checks, concurrent-edit and
duplicate-node refusal remain intact. Browsing must be active and healthy to use
these hints. Manual peers remain supported.

## Native application outcomes

At source `67d5a76`, the positive command exits zero and verifies:

1. An actual Linux controller observes the exact fresh macOS HTTPS node through
   its production discovery API. Anonymous access is refused; discovery does
   not enroll the peer.
2. Chrome on macOS opens the Linux application's Manage servers UI through a
   private SSH forward, chooses that candidate, preserves its HTTPS hostname
   and port, supplies a fresh temporary grant, and explicitly verifies identity.
   The endpoint returns 204; node/account identity is checked and scoped session
   inventory succeeds.
3. With the browser closed, the actual macOS receiver stops and starts with a
   changed HTTPS port and its retained home, key and identity. The production
   controller maintenance loop migrates the endpoint within twenty seconds;
   the pin is unchanged and inventory remains accessible.
4. The actual Linux controller stops and starts against its retained private
   home. The same peer, migrated URL and node/account/key pin remain usable.

The paired `--changed-node-proof` command at the same source deliberately changes
only the owned receiver's stored node ID after its advertisement starts. Its
advertisement and cryptographically proven identity then disagree. Verification
returns 502 and leaves the directory unpinned. The negative command exits one,
retains `passed: false`, and records `changedNodeRefusedWithoutPin: true`. This
is a refusal control, not a successful positive run.

Both runs verify native architectures and transferred executable digests, retain
unchanged source inventories, and stop/wait for their owned applications, browser
and SSH forward. The [public receipt](receipt.json) binds source, binaries and
private logs to the observed outcomes. Raw logs, screenshots, private fixture
homes and complete inventories remain under `_build/discovered-application631/`
in the primary checkout. Screenshots and LAN records are not published.

The earlier `9cc8afd` positive also passed. Two setup failures are separately
retained: `8ffebf8` tried to read a tracked directory symlink as a file before
launching any application; `7ce96e2` opened the manual Add server editor instead
of the directory containing discovered candidates. Neither is product-success
evidence. Inventory now hashes symlink text without following it.

## Other checks

The complete HTTP API race suite passes at production source `9cc8afd`: 296
cases/subcases, nine opt-in skips and 391.222 seconds. Later qualifier/receipt
commits change no Go production or test bytes. API vet, all five maintained
strict TypeScript projects and the final tools-project check pass. Focused
hostname/CA/grant refusal and same-connection proof/bearer controls also pass,
alongside malformed, stale, foreign, plaintext and unsafe-address hint controls.
The paired native application commands use the same executable hashes.

## Reproduction and limits

Use an already authorized native Linux ARM64 SSH host on the same multicast
network, existing Node/Go/Chrome/OpenSSL tools, verified non-loopback LAN
addresses and interface names, and an existing private output parent. Start
from a committed clean checkout; every output leaf must be new:

```sh
GOMAXPROCS=4 GOFLAGS=-p=2 node webmux/scripts/qualify-discovered-application.mts \
  --output /YOUR/PRIVATE/NEW/OUTPUT --peer YOUR_AUTHORIZED_SSH_HOST \
  --local-bind YOUR_MACOS_LAN_IP --remote-bind YOUR_LINUX_LAN_IP \
  --local-interface YOUR_MACOS_INTERFACE --remote-interface YOUR_LINUX_INTERFACE
```

The Linux controller alone receives its fixture certificate through
`SSL_CERT_FILE`; TLS verification is not bypassed. The private key remains in
the owned macOS fixture home. Passwords/grants are newly generated, both homes
require local-account authentication, and no installed account or credential
store is copied. Add `--changed-node-proof` in a separate fresh output namespace
for the expected-failure control. Existing output is refused before any build or
remote command.

This is native Linux ARM64-controller/macOS ARM64-receiver application evidence,
using IPv4 LAN discovery and HTTPS plus a macOS Chrome client. It does not prove
installed-service upgrades, OS trust provisioning, native Windows/Intel/IPv6,
full isolated-network or spoofing coverage, ordinary/managed real-agent
participation, MAC admission/publication or the complete #664/#631 workflow.
No installed service, OS trust/network policy, existing provider or MAC fleet
state changed. Local-only qualification retains `[skip ci]` and does not start
new hosted jobs.
