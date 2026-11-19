# Adopt the interactive Codex policy guard

The integration candidate already contains the reviewed #539 fix, but this
feature branch did not. Adopt 5a0256d as 475f2ff so original-conversation Resume
cannot silently discard an interactive permission change. See the
[adoption receipt](receipt.json).

The guard reads only the original local journal's applied-settings records,
including changes made without another model turn. It checks before preparation
and again immediately before dispatch. Changed or unavailable evidence refuses
structured Resume, keeps the original identity/logs, and explains manual
continuation. Journal settings never become new launch arguments. A surviving
original process can still reattach with its actual current policy. Ordinary
duplicates do not reopen a handoff or recheck policy.

The policy reader, recovery function and public error mapping are byte-identical
to reviewed source 5a0256d. The runner's relevant recovery action is also identical;
its other differences concern unrelated project-pod discovery. Adoption checks
pass all 261 helpers at default concurrency, including 24 focused policy/recovery
controls, all five strict TypeScript projects, twenty race repetitions of the
public recovery test, full HTTP/coordinator race suites and Windows-target vet.
The new public-error regression fails the unmodified 1a parent because its
fallback does not explain the policy refusal and retained manual choice.

The [original native qualification](https://github.com/jordanhubbard/webmux/blob/4b0d4b0ec699bc6d34b48fa7a9f9911481879557/docs/validation/codex-interactive-policy-2026-11-19/README.md)
remains bound to source 5a0256d. It covers actual native macOS / Rig 0.7.18 / Codex 0.169.0
permission changes with and without a subsequent model turn, refused recovery,
survivor reattachment, and unchanged workspace-write/never recovery with fresh
before/after tool-policy probes. Its three cases pass 40 browser/lifecycle
controls. That is historical component evidence, not a new native run on this
adoption. Existing protocol fixtures inject an approved policy check so their
ownership/duplicate assertions stay unchanged; separate journal controls cover
the real guard.

This adoption starts no native coding session, copies no authentication state,
and changes no policy from journal contents. Final exact-head CI, native ARM64,
review/integration and broader #318/#438/release acceptance remain open. The
earlier 1a full CI and source-bound SYSTEM application receipt remain retained
under their original source, without promotion to this new head.
