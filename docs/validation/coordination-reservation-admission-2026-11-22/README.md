# Shared reservation admission review

This [source-bound receipt](receipt.json) integrates
[#641](https://github.com/jordanhubbard/webmux/pull/641) with the corrected claim,
message and peer-discovery parent for [#630](https://github.com/jordanhubbard/webmux/issues/630).
It adds controlled real-TLS regressions for outgoing sharing revocation during
reservation recovery. All three controls failed against the original resolver.

- An authenticated absence reply is held while the operator disables outgoing
  reservations. Recovery must send no new admission RPC after revocation.
- A status reply for an existing grant is held across the same revocation. Its
  historical result must not return a current, usable lease.
- A first admission commits before its reply is held across revocation. The
  authority may retain that grant, but the source must withhold current-lease
  permission when the delayed reply arrives.

The resolver now reloads current peer permission, mapping, identity and credential
before a new admission, after awaited status recovery and repository inspection.
After either kind of reply it retains the authority's historical outcome but
checks current sharing before returning a usable lease. Revocation marks the
intent authorization-unavailable and clears current verification. In each control,
restoring permission recovers the same intent with one total admission and fence
1; it does not create a replacement grant. An RPC already started can still commit.

The fixtures stop only their isolated source background worker before establishing
fresh peer context. The absence control explicitly persists an attempted marker to
simulate interruption before sending; this is not a new actual crash run. Status
and admission use the real TLS receiver and the ordinary scoped action API.

Validation covers the full Node 24 Go suite, coordination ledger/client/MCP race
checks, scoped HTTP API race checks including these controls and all parent
revocation controls, vet, strict TypeScript, frontend lint, 481 frontend tests, the production build and the
production CLI/MCP HTTP/WebSocket/restart contract.

The [earlier native reservation receipt](../coordination-reservations-2026-11-22/README.md)
remains bound to its original binaries. This review adds no new native LLM,
two-host or heavy-job acceptance. Fresh hosted checks on this head and parent
delivery remain required before merge. Full roadmap acceptance stays open.
