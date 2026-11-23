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
