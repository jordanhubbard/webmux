# Agent Deck version-check cancellation readiness

Qualification for [#636](https://github.com/jordanhubbard/webmux/issues/636). The old fixture started a 300 ms context before its shell had written the child PID. A slow startup could therefore produce `ENOENT` instead of testing cancellation of an established child group.

The maintained test starts the real check asynchronously, waits up to two seconds for an atomically published child PID, then cancels and retains the original two-second cancellation bound and child-survival assertion. A deliberate one-second startup delay rules out the former scheduling assumption. Early failures cancel and wait for the owned check. The production three-second version-check budget, 200 ms wait delay and process-group kill are byte-identical to the base.

Five full package race repetitions and vet passed on macOS ARM64. A Go overlay restoring just the old pre-readiness 300 ms context fails before child readiness, as intended. This discriminating control does not claim the historical scheduling interleaving. The [receipt](receipt.json) separates local evidence from pending native Linux CI. No installed runtime, service or NAS state changed.
