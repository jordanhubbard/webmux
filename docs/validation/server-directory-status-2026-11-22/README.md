# Server-directory status during policy loading

Qualification for [#680](https://github.com/jordanhubbard/webmux/issues/680).
The [receipt](receipt.json) records exact sources, the original Windows failure,
a local regression control and subsequent local checks.

The server manager has independent directory and coordination-policy statuses.
Windows CI at #655's head failed because a generic status locator matched both.
Give the directory notice the accessible name `Server directory status` and
scope directory save/connection assertions to it.

The existing real two-server browser scenario now holds the backend's actual
policy response until directory save and connection feedback are verified.
Policy loading stays visible throughout those operations. The original generic
locator fails deterministically with Saved and Loading statuses; the named
locator passes. Cleanup releases and settles held responses before removing
the route, preserving the original assertion on failure. No sleep or fabricated
policy response supplies this coverage.

Strict TypeScript checks, the full build, and both federation/policy browser
scenarios pass locally (2 tests, 28.8 seconds). The Playwright browser bundle was
missing, so the tests used installed Google Chrome 158.0.8127.36 with an isolated
profile. Earlier missing-browser, grep-selection and temporary barrier-cleanup
failures are retained separately and do not count as product regression proof.
Native Windows recovery remains pending new-head CI; local success is not that
target evidence. No installed service, user browser profile or NAS state changed.
