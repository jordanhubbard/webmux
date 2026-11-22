# Cross-node message admission follow-up

This [source-bound receipt](receipt.json) integrates the corrected peer-discovery
parent into [#639](https://github.com/jordanhubbard/webmux/pull/639) for
[#630](https://github.com/jordanhubbard/webmux/issues/630). Two controlled real-TLS
regressions exposed production admission bugs and failed before their fixes.

A recovery attempt could obtain authenticated absence, wait for that reply, and
send a new admission after outgoing sharing had been revoked. The worker now
resolves current sharing permission, mapping, pinned identity and credential again
immediately before starting admission. The regression holds the status reply,
revokes permission through the operator API, and requires zero new admissions.
Restoring permission then admits the same message exactly once. Its durable
attempt marker simulates an interrupted attempt; this is not a new crash run.
An RPC already started before revocation can still commit.

An 8 KiB text containing literal `<` characters fits raw agent API input, but Go's
JSON encoding expands that text to about 48 KiB on the federation wire. The old
16 KiB receiver bound left the locally accepted message undelivered. The receiver
now allows a bounded 64 KiB envelope, including escaped text and bounded identity
fields. The regression verifies exactly one complete recipient inbox entry and
also requires refusal of 8 KiB plus one byte. Semantic field limits are unchanged.

Final validation passed: full Node 24 Go tests, scoped HTTP API race tests, vet,
and the production CLI/MCP HTTP/WebSocket/restart contract. Core ledger race
checks, strict TypeScript, frontend lint, 480 frontend tests and the production
frontend build also passed; their source was unchanged by the final receiver
budget adjustment. Private logs include the original failures and final passes.

These tests use isolated controllers and ordinary PTY fixtures. They establish
neither new native LLM collaboration nor new two-host acceptance. The earlier
[native protocol receipt](../coordination-messages-2026-11-21/README.md) remains
bound to its original binary. Fresh hosted checks and parent delivery remain
required before merge. Full roadmap acceptance stays open.
