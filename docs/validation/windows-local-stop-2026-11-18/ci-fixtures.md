# Browser fixture lifecycle after explicit local Stop

The #541 Delete guard exposed browser-test cleanup assumptions. CI
[44453505091](https://github.com/jordanhubbard/webmux/actions/runs/44453505091)
failed because unconfirmed Windows local providers retained their saved sessions,
while tests ignored DELETE status and selected an unscoped Session tools button.
The mobile test also retained a browser worker whose connections prevented its
loopback origin from closing.

The native-provider startup warnings predate this change. Successful baseline
[44449205787](https://github.com/jordanhubbard/webmux/actions/runs/44449205787/job/135308708838)
contains the same unconfirmed-startup warnings, including a passing mobile case.
Their cause and an application gate that requires an actually connected Windows
local shell are tracked separately in [#543](https://github.com/jordanhubbard/webmux/issues/543).
This fixture repair does not resolve that issue or establish native conversation
recovery. The [native application qualification](README.md) supplies separate
evidence on the private Windows worker.

Browser fixtures now scope terminal controls to their selected session and end
only the browser lease issued to that fixture's viewer. Return to terminal can
release the lease while keeping the worker alive. Cleanup checks availability
and can claim an available lease as the same viewer; it never forces takeover
from another viewer. A local test reproduced the released-lease conflict before
this correction; that failed run remains retained.

Terminal cleanup reviews the current session. For the local lifecycle contract,
it freezes the original selection and UUID, or inspects the existing operation.
Deletion follows a matching confirmed Stop. Unavailable owners, lost replies and
uncertain completion retain the saved session, minimize it and attach an explicit
retention receipt. No cleanup path automatically continues an uncertain Stop or
substitutes another provider. Other lifecycle owners keep their deletion route.

Before a new Windows fixture allocation, the previous complete home is renamed
to an ignored retained directory instead of being recursively removed. This
preserves recovery keys, state, logs and the original executable when an
unconfirmed owner may outlive its controller. A rename failure stops allocation;
there is no destructive fallback. Unix fixtures retain the outer runner's
existing private tmux cleanup. Retained homes contain private fixture state and
are not included in the public validation receipt.

All eleven affected real Go/Chrome browser cases pass locally. Eleven focused
controls pass on both macOS Node 24 and native Windows Node 26.13.0, including
lost/uncertain replies, foreign responses, original-only inspection, released and
foreign browser leases, and full directory retention on Windows. The first
browser run's ten passes and released-lease failure are retained alongside the
corrected eleven-case run. Strict checking covers all five TypeScript projects.
The [receipt](ci-fixtures.json) binds sources and raw evidence, including the full
helper-suite result. No production ownership rule, existing browser assertion,
seed, count or deadline is relaxed. Hosted CI on the final source remains a
separate gate.
