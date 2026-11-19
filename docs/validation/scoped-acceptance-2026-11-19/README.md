# Integrated fixture and USB acceptance audit

Audited main `4f4bf33acc4ed57275a1c33bb649bdd1bb35dbb0` against the five
issues below. Their earlier reports stopped at pending native checks or
integration. Those scoped requirements are now complete; the historical reports
and original failures remain unchanged.

| Follow-up | Verified acceptance |
| --- | --- |
| [#535](https://github.com/jordanhubbard/webmux/issues/535#issuecomment-7023712021): Windows status reader | Original sharing violation retained. Native ARM64 control forces the old absolute-reader failure while the production root reader preserves complete status and metadata. Original corrected-source full CI passes, including server race/vet on Linux, macOS ARM64 and Windows AMD64. Both native ARM64 cases also pass after composition. Relevant current sources are byte-equal. |
| [#524](https://github.com/jordanhubbard/webmux/issues/524#issuecomment-7023753094): Rig wheel history | Larger-history control fails after 20 real wheel events and passes after 38, within the existing assertion deadline, with unchanged workspace coordinates. The final uninstrumented test restores 40 fixture lines. Its complete wheel/control section is byte-equal on current main; the published Rig 0.6.14 Linux suite passes all eight cases. |
| [#525](https://github.com/jordanhubbard/webmux/issues/525#issuecomment-7023794075): Rig dashboard fixture | Original wrong-endpoint/stopped-daemon failure retained. Restoring the old worker teardown causes the maintained control to reject two failures. Integrated global teardown and explicit endpoint binding preserve the original daemon across worker replacement, complete the unchanged dashboard flow and clean up afterward. Native Linux ordinary suite and lifecycle control pass together. |
| [#505](https://github.com/jordanhubbard/webmux/issues/505#issuecomment-7023897627): USB relay process pipe | Bare and wrapped process-pipe closure controls fail against the parent. The established relay accepts that sentinel while malformed, truncated and permission failures remain errors. Native Windows real-SSH race repetition passes three times. The original SSH cancellation/host-key tests and current relay sources are preserved. |
| [#459](https://github.com/jordanhubbard/webmux/issues/459#issuecomment-7023898654): USB fixture readiness | Original sysfs-before-devnode failure retained. Exact-payload readiness and all original portable test methods are preserved; 13 current portable controls pass. Original and later integrated native virtual-kernel artifacts admit the expected unpatched negative, corrected held-reply case, 20 direct cycles and WebMux/SSH lifecycle. |

The [receipt](receipt.json) records original sources, run/artifact identities,
raw digests and the scope of source equality. Native execution remains attributed
to those original commits. Source equality is not a fresh current-main native
execution or an independent signed VCS attestation. No new hosted qualification
jobs were dispatched; only existing results were read. Original failed CI,
dependency-audit failures and deliberately failed control children retain their
actual dispositions.

These closures do not qualify authenticated recovery, every peripheral/backend,
all grid/policy/platform combinations or the release. In particular, the private
Windows VT deadline under #536 still fails on changed source and its cause
remains unresolved. Broader #318, #438, #194 and release gates remain open.
