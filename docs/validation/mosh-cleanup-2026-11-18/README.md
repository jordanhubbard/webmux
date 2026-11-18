# Native Mosh cleanup diagnosis

Combined source `45151c356368b52eb20ccea92a34866472041e31`, Linux CI
`44273583502`, passes the real Mosh sign-in, private SSH callback, logging-off,
no-URL and acknowledgment controls, but reports `providerStopped: false`.
The final boolean assertion hides the earlier caught cleanup error. Artifact
`14415120434` retains that failure; it is not a feature or cleanup acceptance.

The diagnostic reports caught cleanup errors before the final boolean assertions
and provides the first as an explicit `AggregateError.cause`. The installed
Playwright runtime serializes `cause`, while it does not serialize the aggregate's
`errors` array for runtime failures. A public Playwright CLI expected-failure
control confirms that the cause survives in its structured reporter. The native
tmux socket assertion also reports its kill-command exit/error code, without
changing its required absence result or five-second deadline.

A separate Linux workflow builds the exact application and uses the same verified
tmux 3.4 source as full CI. A fixed cohort of 20 independent invocations gives
each sample a fresh backend and private fixture state, avoiding production request
budget accumulation. Each has a unique artifact directory. Any failed sample
fails the aggregate. All original sign-in/privacy/acknowledgment/cleanup assertions,
per-test deadlines, zero retries and the twelve-minute diagnostic job limit remain.
These are independent diagnostic samples, not retries until success.

Source `169eb6d`, which first improved error order, passes a single native Linux
sample in run `44278236979` and actual macOS Mosh locally. That later passing
sample does not explain or repair the original failure. Its complete original
full-CI context is also being inspected in `44279746826`. The additional explicit
cause and fixed-cohort changes pass strict typechecks and the public reporter
control; their native cohort remains pending. No production behavior, required CI
gate, cleanup condition or deadline is weakened. Keep #468/#452 open.
