# Native collaboration layer integration review

This [source-bound receipt](receipt.json) integrates
[#643](https://github.com/jordanhubbard/webmux/pull/643) with the corrected reservation,
claim, message and peer-discovery parent for [#630](https://github.com/jordanhubbard/webmux/issues/630).
The source merge is clean. It retains current account authorization, compact
Codex policy parsing, peer-cache scope identity, message admission permission and
encoding bounds, conservative claim timing, reservation reply authorization and
the independently qualified Rig cancellation fixture repair.

Review covers exact-version native support, preservation of explicit invocation
policies, cursor-based read-only MCP waits, repeated private access loading,
authority refusal, cancellation and rolling-output capture. Wait configuration is
0–30 seconds; its deadline permits at most one further bounded scoped read, as
stated in the original receipt. Reading never acknowledges a message or executes
work.

Current combined-source validation covers the full Node 24 Go suite, core ledger,
client and MCP race tests, scoped HTTP API race tests including output capture and
all inherited revocation controls, vet, strict TypeScript and the production
CLI/MCP HTTP/WebSocket/restart contract. This layer changes no frontend component.

An offline verification matched all 29 retained artifact hashes from the
[original native receipt](../coordination-native-pair-2026-11-22/README.md). Its
selected native conversation audit passed again. All three altered-artifact
controls still refuse their corresponding missing busy result, incorrect guarded
job fence and missing handoff completion. Original failed attempts remain failed.
This re-audit establishes retained evidence integrity; it does not run native
agents against the current combined binaries.

Fresh hosted checks and parent delivery remain merge gates. Current-binary native
workflow qualification, the full discovered/managed/SSH/reconnect workflow and
broader roadmap acceptance remain open. No installed service, existing user agent,
account policy or original private artifact was changed by this review.
