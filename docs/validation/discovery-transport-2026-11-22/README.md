# Native multicast transport diagnosis

Issue [#631](https://github.com/jordanhubbard/webmux/issues/631) remains open.
The same production socket setup receives exact self-originated multicast DNS
queries on Linux physical interfaces, but fails that control on this Mac's two
physical interfaces. This narrows investigation below DNS-SD candidate parsing;
the cause of the Mac's reception failure remains unproven. No product socket
binding, service, OS network policy, assertion or existing deadline changed.

The opt-in `TestMulticastTransportControl` uses production `multicastSockets`,
joins a specifically selected interface and sends a unique, valid PTR question
to the real mDNS group/port from a separate native UDP socket. It checks the
sender's existing multicast loopback setting and requires receipt of the exact
wire question within one second. Ambient packets cannot establish success.
It probes both address families, at most four named interfaces, sequentially.
It does not browse candidates, authenticate peers or prove inter-host reachability.

Run from `webmux/server`, choosing actual interface names for the target:

```sh
WEBMUX_TEST_MDNS_CONTROL=lo0,en0,en1 go test -count=1 -v \
  -run '^TestMulticastTransportControl$' ./internal/discovery
```

A failed selected interface/family makes the command fail. No interface fallback
or unsupported-family skip converts that result to success. Loopback is an
explicit transport control here; product discovery continues to exclude
loopback candidates. Without the environment variable the diagnostic is skipped.

| Native target | IPv4 loopback | IPv6 loopback | Physical IPv4 | Physical IPv6 |
| --- | --- | --- | --- | --- |
| macOS 27.2.1 ARM64 | Pass | Receive timeout | Both timeout | Both timeout |
| Linux ARM64 | Pass | Send: network unreachable | Both pass | Both pass |

Both aggregate diagnostic invocations exit 1; the table preserves the individual
results rather than labeling either complete run green. On macOS each failing
case successfully joins and sends before reception times out. Linux's IPv6
loopback failure is a send failure and remains distinct.

The unchanged `TestMulticastAdvertisementRoundTrip` and
`TestNodeMulticastRoundTrip` were also rerun. Both fail on macOS: the legacy
resolver returns default port 8443 instead of advertised 19443, and node discovery
times out after the original eight seconds. Both pass on Linux with the
source-bound binary; the separate transport diagnostic is skipped in that
invocation. The Linux multicast tests themselves do not skip.

An initial Python direct-bind experiment suggested the Pion bind constants might
be wrong. Go's `net.ListenUDP` actually rewrites multicast binds to wildcard
listeners and enables sharing, so that experiment was not equivalent to WebMux.
Actual Go socket controls supersede that hypothesis. No binding patch was made.

[The receipt](receipt.json) binds source files, the Linux binary and complete
retained logs, including failed commands and their exit codes. Discovery race
tests and vet pass. Linux and Windows ARM64 compilation passes; Windows has no
native transport result here. No installed server or active agent was restarted.

Complete discovered/enrolled two-host acceptance, supported-platform coverage,
address changes and the remaining #631 controls still require qualification on
a suitable network. These controls neither replace nor complete those gates.

## Corrected-parent integration review

The [integration receipt](integration-receipt.json) records review against the
corrected native collaboration parent. Discovery race tests, vet and Linux/Windows
ARM64 test compilation pass. All seven source/dependency hashes match the original
receipt. The diagnostic remains opt-in; there is no new native multicast run or
product transport change. The original failed native results remain failed, and
#631 plus full discovery/enrollment acceptance remain open. Fresh hosted checks
and parent delivery are separate merge gates.
