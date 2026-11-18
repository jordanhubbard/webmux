# Native capture and prepared recovery composition

Preparation source `5961ab8140c82f6c4ac57d96416aa3ccf4bc882d` composes published #511 `d41ac09`
onto canonical `951cf93`, retaining every prior input commit. It adds the
internal original-conversation capture action and separates exact reviewed
request preparation from native dispatch. Public recovery API/UI remains open.

All 226 helper tests pass with zero skips, all five strict TypeScript projects
pass, and the affected coordinator and HTTP API packages pass Go race tests and
vet. No frontend source or production browser test changed in this preparation.
The composition retains #522's broadcast observer fix and #527's shared-daemon
lifecycle fix. Source #511's Linux CI failed its older immediate broadcast
assertion; that failed run is retained, without attributing it to capture.

The published capture receipt's 18 unchanged sources match; its resume module
has the separately published later preparation change. All 11 prepared-recovery
module hashes match this composition. Both retained raw receipt hashes were
verified. These remain proofs for their named component revisions, not a fresh
native capture/recovery result for this merged runtime. Prior fresh composed
creation/coding/checked-Stop proof belongs to `10295c9` under the
[public native composition receipt](../roadmap-public-native-composition-2026-11-18/README.md).

See [receipt.json](receipt.json) for source hashes, checks and remaining scope.
PR #500 stays at `951cf93` while its original hosted handles finish. This
preparation needs its own hosted and composed native acceptance before promotion.
No main, version, tag or release publication is changed.
