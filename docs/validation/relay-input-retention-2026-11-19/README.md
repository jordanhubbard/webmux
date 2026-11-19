# Relay input ancestry and historical evidence retention

This merge retains the original #547, #551 and #553 input commits in #452's
canonical ancestry and restores the two historical #547 documents unchanged.
The base is `b0b3f3545203e461fd4ef79d5059c9d6c597b46c`; the incoming head is
`14ad699b57b62ef556f4e76b74362d447bd8e6b1`, which already contains the other two
heads. Only documentation differs from the canonical base. This is not a new
native execution or a repair claim for an original failed run.

## Conflict review

The canonical scripts already retain the public-CA exporter and #553's final
HTTPS validation records. Its newer `run_recorded` wrapper preserves partial
stdout/stderr, rethrows timeouts and retains nonzero results, while adding
bounded streams, command identities, stage evidence and start-error records.
The old `run_logged` implementation must not replace that reviewed successor.
The canonical tests retain the real-subprocess timeout/partial-UTF8 and ordinary
result controls, adapted to the newer event schema, with additional trust-store
refusal controls. `write_build_inputs` remains identical to #553.

Both workflow conflicts retain the canonical files unchanged: the APT workflow
runs both relay control suites, and selected signed Azure sources retain the
canonical limits and refusal controls. The old CI insertion would duplicate a
legacy test invocation from the earlier stack; the canonical APT workflow
already executes the current control suite. No production check, package,
assertion, deadline, source selection or trust policy changes in this merge.

## Evidence boundaries

The restored [historical report](../rig-relay-timeout-evidence-2026-11-19/README.md)
and its receipt describe their original source and failed build, not this merge.
The source-bound b0 and 9f qualifications remain separately labeled; unchanged
runtime trees do not relabel their revisions. The incoming HTTPS report is
already byte-identical in the canonical base. The [retention receipt](receipt.json)
records source-tree and restored-document bindings. Full #318/#438 acceptance,
original failure attribution, exact-main gates and release remain separate.
