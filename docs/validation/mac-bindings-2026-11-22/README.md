# Versioned MAC project binding foundation

Source-bound local qualification for [#673](https://github.com/jordanhubbard/webmux/issues/673).
Read the [contract](../../mac-project-bindings.md) and [receipt](receipt.json).
This is a parser/resolver foundation, not a deployed MAC bridge or live read adapter.

The account and exact verified repository select one explicit operator mapping.
Foreign accounts, changed repository/root witnesses, unsupported capabilities,
ambiguous or duplicate entries, duplicate/escaped JSON keys, unknown fields and
credential-bearing or unsafe origins are refused without echoing policy data.
Revision and copy controls protect later scoped observation caches.

Local package race tests, vet and a bounded two-worker fuzz exercise pass.
Separate deliberate Go overlays remove account selection or duplicate-key
refusal. The expected foreign/unknown-account and duplicate/escaped-key controls
fail in those mutants; the real source remains unchanged. These are mutation
controls, not historical defects, prior source revisions or live fleet evidence.

The current MAC main source was refreshed by pinned GitHub reads at
`fa08464470f6562ae53329c8e4385e87e9061856`. The inspected API and project-graph
contracts remain unchanged from the earlier source audit. No MAC checkout or NAS
was accessed, and no task, worker, credential or fleet policy was changed.

Remaining #673 mappings and actual authority qualification, human/agent read
consumers, shared admission/publication and three-host acceptance remain open.
The observed local checks do not qualify those requirements or current-head CI.


## Fixed read-route qualifier and exact-field correction

The later [read-access receipt](read-access.json) binds the current source
separately from the first parser-only checkpoint. A Go helper now checks absent,
invalid and supplied credentials through one fixed GET route, with TLS, redirect,
proxy, response-bound and cancellation controls. It exposes only observation
metadata and leaves principal identity and non-read operations unqualified. No
product loader, browser/MCP consumer or live fleet configuration invokes it yet.

The work also corrects Go JSON's case-insensitive struct-field matching. Operator
policy fields are now checked by exact name before decoding; uppercase or
case-alias fields cannot silently select another account. Upstream evidence uses
exact wire-key lookup as well. Deliberate missing/invalid-credential and case-alias
mutations fail the intended controls; the actual source remains unchanged.

Private profile metadata was inspected locally without reading credential files.
No suitable current read-only profile was found. Controlled HTTP/TLS tests do not
qualify the real fleet, actual least-privilege scopes, verified SSH publisher
ownership or live revocation. Those and the remaining phase-one wire mappings
stay open under #673. No MAC or NAS mutation was performed.
