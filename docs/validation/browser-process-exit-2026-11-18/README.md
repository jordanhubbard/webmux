# Wait for owned Windows browser processes before private-file removal

This is a proposed repair under [#473](https://github.com/jordanhubbard/webmux/issues/473),
stacked on diagnostic #510 (`1a131ca526a1edddcf24128d56d20ecf3c8ea894`). Native Windows
qualification of this repair is required before claiming completion.

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
