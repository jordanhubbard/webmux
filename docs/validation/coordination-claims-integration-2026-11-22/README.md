# Advisory claims integration review

This [source-bound receipt](receipt.json) integrates
[#640](https://github.com/jordanhubbard/webmux/pull/640) with the corrected message
and peer-discovery parent for [#630](https://github.com/jordanhubbard/webmux/issues/630).
It retains account-removal authorization, compact native Codex policy parsing,
peer-cache authorization periods, the pre-admission sharing recheck and the
bounded envelope allowance for JSON-escaped message text.

The automatic source merge exposed a compile error: the claim layer needs the
network request start timestamp that the cache authorization fix no longer uses.
Restoring that observation before network I/O preserves conservative remote lease
aging. Cache validity continues to use authorization scope identity, independently
of this timestamp. The initial compile failure is retained in the private evidence.
The documentation conflict was reconciled to retain both the admission follow-up
and the claim protocol description.

Validation covers the full Node 24 Go suite; coordination ledger/client/MCP race
checks; scoped HTTP API race checks for claims and the parent account, peer-cache
and message regressions; vet; strict TypeScript; and the production CLI/MCP
HTTP/WebSocket/restart contract. No frontend component changes in this layer.

The [earlier native claims receipt](../coordination-claims-2026-11-22/README.md)
remains bound to its original binaries. This review adds no new native LLM,
two-host or LAN acceptance. Hosted checks on the final head and parent delivery
remain required before merge. Complete real-agent negotiation, shared worker
acceptance and the broader roadmap remain open.
