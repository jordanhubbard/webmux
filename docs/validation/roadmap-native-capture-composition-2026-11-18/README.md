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
verified. These original receipts remain proofs for their named component revisions. Prior fresh composed
creation/coding/checked-Stop proof belongs to `10295c9` under the
[public native composition receipt](../roadmap-public-native-composition-2026-11-18/README.md).

See [receipt.json](receipt.json) for source hashes, checks and remaining scope.
PR #500 stays at `951cf93` while its original hosted handles finish. This
preparation still needs its own hosted qualification before promotion.
No main, version, tag or release publication is changed.


## Fresh composed native acceptance

Actual Go/Chrome/native Codex 0.169.0 on macOS arm64 now passes public creation,
discarded-reply and duplicate controls, a fresh coding turn and two calls to the
embedded-Go original-conversation capture action. Both captures select the same
native conversation and reuse the same snapshot. Checked Stop preserves its
viewer and sibling, and owned controller/browser/daemon/tmux cleanup passes.

A separate fresh fixture passes native original-only preparation and recovery.
Two separate Go invocations bank and reread the exact reviewed request without
POST, dispatch marker or environment handoff. Explicit completion first posts
the banked operation; its response stays uncertain, an ordinary duplicate posts
nothing, immediate explicit inspection remains operation-in-progress, and later
inspection confirms the original conversation. The native conversation recalls
the removed challenge without tools. Both recorded witness byte prefixes occur
unchanged in the final conversation file, with appended records. Fresh correlated
tools preserve the default owner policy, model, environment and allowed writes.
Duplicate recovery retains provider/sibling PIDs; checked Stop and cleanup pass.

Both runs pin/report actual controller Node 24.21.0 and refuse native trust
prompts. Their native records contain no delegation calls, and durable coordinator
files contain no plaintext arbitrary fixture environment witness. Private raw
records, adapted drivers, owned fixture files and the exact controlled native
conversations are retained separately by hash. Neither fixture copies account
authentication. The browser is loopback auth-none and the native account remains
in place. These are separate capture/preparation proofs for runtime `5961ab8`;
public recovery API/UI and actual controller crash/reboot are not qualified here.
