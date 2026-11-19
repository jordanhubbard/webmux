# Federated renderer capability acceptance

[#484 is closed](https://github.com/jordanhubbard/webmux/issues/484#issuecomment-7024471954)
after auditing main `567f0e99b19ba706543c96e511f842424af970c5`. The
[receipt](receipt.json) records the local controls, original native source and
artifact bindings, current source equality and limits.

The proxy forwards only the first `snapshot_profile` selection to the configured
peer's fixed terminal route. Browser credentials, cookies, Origin and arbitrary
query keys are excluded. The existing real-WebSocket test passes locally under
Go 1.27.4 race detection. A counterfactual removing only the forwarding block
fails all three nonempty-profile query assertions; the original logs remain
retained. A concurrent two-file documentation merge triggered the local driver's
final HEAD guard. All tracked code was byte-equal across that transition, so no
test rerun or relabeled exact-whole-head result was needed.

Native Windows run [44541552813](https://github.com/jordanhubbard/webmux/actions/runs/44541552813)
on composed source `0056000` passes the actual ConPTY renderer-required owner
test without failures or skips. It preserves the original owner and shell PID,
refuses absent/unknown/older-v1 viewers, negotiates v2 replay, recaptures state,
continues input/output, retains one provider and refuses another owner. The
fixture child is synthetic; this is WebSocket/native-provider qualification,
separate from browser pixels or authenticated coding recovery. Full original
source CI also passes before the reviewed integration.

All four federation implementation/test files, native producer/workflow,
native-provider subtree and screenstate subtree are byte-equal between that
qualified revision and audited main. Native execution remains attributed to its
original source; equality is not a fresh current-main native execution or an
independent signed VCS attestation. The earlier `95f2542` result retains its own
source scope; this audit uses the later composed test with augmented v2 coverage.

No new hosted or native Windows jobs were started. The capability correction
does not establish the historical blank-rendered-replay cause under #478.
Broader screen restoration, recovery and full roadmap/release acceptance remain
open.
