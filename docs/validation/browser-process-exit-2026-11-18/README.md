# Wait for owned Windows browser processes before private-file removal

This records the cleanup repair under [#473](https://github.com/jordanhubbard/webmux/issues/473),
originally stacked on diagnostic #510 (`1a131ca526a1edddcf24128d56d20ecf3c8ea894`).
Scoped acceptance completed on 2026-11-19 after integration. The final audit
below retains the earlier proposed behavior, original failures and their limits.

## Established evidence

The fixed diagnostic [run 44309028494](https://github.com/jordanhubbard/webmux/actions/runs/44309028494)
completed all 100 observed samples and failed cleanup in trials 32, 35, 98 and
99. Every sample loaded, requested and navigated once. Artifact `14430880126`
contains all 150 receipts, SHA-256
`10fc2480df8a9aa0f92a5805d2a7738cd86a75ccd4cbcdd1fac8d058184a0e3d`.

Trial 35 found a creation-time-validated owned browser child still holding the
exact failed cache file after the parent exited. All nine process identities
were verified before shutdown; one remained alive at handle-query time, and its
snapshot contained a matching volume/full 128-bit file identity. Restart Manager
reported zero holders. Some handle duplications raced or were refused; this is
positive membership evidence, not complete handle enumeration or identification
of the particular deny-delete handle. The other failures' zero matches do not
exclude a transient holder. The ordinary unobserved run also failed the original
cache-sharing refusal on this source.

A separate native controlled test passed without skip for actual deny-delete
handle membership, creation-time mismatch refusal, a distinct unheld file,
enumeration bounds and survival of the original source handle. These results
support fixing the gap between parent exit and owned-child exit; they do not
constitute a production repair by themselves.

## Proposed behavior

Before closing Chromium, the Windows worker obtains the private browser's CDP
process set, validates the parent, image and creation ordering, and retains
synchronization handles after rechecking exact creation identities. The set is
bounded to 64 unique processes. It never finds processes by image alone or acts
on a newly resolved PID after cancellation.

Allocator cancellation and the existing saved-profile graceful close happen in
their original order. The worker then waits on the retained native process
handles before attempting a single private-profile/transfer-directory removal.
The CDP collection and event-wait phases each have a two-second bound; the
existing five-second graceful-close bound and enclosing deadlines are retained.
This adds no removal retries or arbitrary sleep, and does not terminate an
unrelated process. If shutdown cannot be verified, it returns a fixed error and
retains private files. Real removal errors are also returned without raw paths.

The controller now retains the worker's exit status and reports a nonzero exit
to the authorized End browser request with a fixed message. It does not relay
worker stderr or arbitrary provider errors. A regression control checks both
success/failure and controller denial.

The native first-navigation fixture uses the same shutdown helper. Its original
graceful-close assertion and one-shot removal remain gates; an unverified exit
fails cleanup and retains the profile. Windows receipts record successful exit
verification. Existing navigation modes, sandbox controls and request/navigation
assertions remain. The Windows workflow's controlled gate additionally requires
a real child-process/file test before any manually requested fixed cohort:

- A selected child holds a file that refuses deletion. Wrong creation identity,
  wrong image, duplicate identities and an oversized set must be refused.
- An event-wait deadline must fail while preserving that child's original file
  handle. After that child exits, its retained synchronization handle must signal
  and profile removal must succeed.
- A sibling running the same executable must remain alive with its separate file
  still held until the fixture explicitly releases it.

## Local validation

Passed: all 31 server packages with `go test -race ./...`; host and Windows AMD64
browser-package vet; Windows AMD64/ARM64 test cross-compilation; native macOS
Chrome browser-package race tests including real saved-cookie persistence and
controller/worker-exit controls (28.181 seconds); workflow control-gate parsing;
and `git diff --check`.

Retained local native-browser log SHA-256:
`1dde8c7bf5f74b07eb82efcc19c05029a5898994d0365d732f138f361b002d26`.
Full server race log SHA-256:
`2cdf8e2dd98a98da45e87d4d99d10976f8435b6a607d41c90088fba0206f7c0d`.

Cross-compilation and macOS results do not prove the Windows exit-wait path.
Require the native child/sibling/refusal control and fresh exact-source browser,
profile, ordinary/observed cleanup and normal CI evidence. A passing sample set
alone does not erase the retained failures or establish all possible Windows
cleanup causes. Audit #429 and the other roadmap/release gates remain separate.

## First native Windows result and trace-lifetime correction

At `43016312ff368f8b8512bfb7174e5d7d8ba45d36`, the native selected-child/sibling/
identity/deadline control passed. The fixed 100-sample observed cohort also passed
its step: every sample loaded/requested/navigated once, verified exit, and passed
one-shot profile removal. The independent process observer saw all eight children
exited in 99 samples and all seven exited in the other; none were still running
or unqueryable. This is paired with the retained diagnostic source's four failures
in 100 samples and actual file-membership evidence, not a passing rerun alone.

[Run 44314149768](https://github.com/jordanhubbard/webmux/actions/runs/44314149768)
is nevertheless terminal **FAILURE**: eight ordinary/trace-sample records failed
shutdown verification in the two modes that begin tracing before target creation.
All 150 records remain in artifact `14431414653`, SHA-256
`1ba9b53f8d8a1b4b3958185a3f4afe704d6bc63b2a4cd7760be273f719891bcf`.

The SDK's `ListenBrowser` allocates Chromium on its first call using that call's
context. The early-trace helper had used the cancellable listener context; ending
trace collection therefore stopped Chromium before owned shutdown could verify
it. Bind allocation/listening to the stable root and use the listener context as
a callback guard instead. This retains tracing before target attachment and adds
no navigation. A native regression checks that finishing the first trace leaves
the original browser serving protocol work and able to attach its first target.
It fails against the prior listener ownership and passes with the correction.

The complete local native browser race package passes with this correction
(log SHA-256 `fb333769289985f19b0dd01fd3a587f4aa9b006c71fdfba8f3cb3793677c5a24`),
as do Windows AMD64/ARM64 test cross-compiles. The production shutdown, worker,
manager and native process-wait control files are unchanged from the 100-sample
source. Fresh hosted qualification of the trace correction remains necessary;
the old run's overall failure is retained and is not relabeled green.

## Integrated acceptance — 2026-11-19

[#473 is closed](https://github.com/jordanhubbard/webmux/issues/473#issuecomment-7024309888)
after auditing main `d8159018d301dba60ee2e873e0b3bac2b0d75346`. The
[acceptance receipt](acceptance-2026-11-19.json) binds five original native
packets and the relevant current source equality:

- The original `8e90c3b` profile-removal failure follows a successful single load,
  request and navigation. The diagnostic `1a131ca` cohort retains all four failed
  trials and the exact-file membership observation described above.
- The `4301631` runtime repair passes all 100 observed cleanup samples with all
  independently observed children exited. Its enclosing run remains failed for
  eight separate early-trace helper ownership errors.
- The `dea22c3` trace correction passes all 50 native records: 46 single-request
  loads and four confirmed cancellation controls, with zero cleanup failures.
  Actual native selected-child/sibling/identity/deadline, held-file/refusal and
  trace-owner-lifetime controls pass without skips. The old trace-helper
  negative remains retained.
- Later integrated main `84a331a` has successful full CI and Windows browser
  qualification. Its 23 component receipts contain 22 single-request loads and
  one confirmed cancellation, with zero cleanup failures.

The six production/process-control files are byte-equal from the 100-sample
source through the trace correction to audited main. Relevant navigation and
trace-helper sources also match the later integrated native qualification.
Current-source equality does not relabel those original native executions as
fresh current-main executions or constitute independent signed VCS attestation.

No new hosted jobs or native sampling were started for this audit. This closes
the scoped owned-child cleanup correction after review and integration. It does
not establish every historical navigation cause under #282, authenticated
recovery, broader platform/security coverage or full roadmap/release acceptance.
