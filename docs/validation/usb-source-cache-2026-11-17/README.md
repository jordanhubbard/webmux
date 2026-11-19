# USB fixture source-cache candidate

Issue #445 addresses repeated downloads of the same eight pinned Linux USB
fixture sources on fresh CI runners. In exact c7d349c USB
job [134455210533](https://github.com/jordanhubbard/webmux/actions/runs/44201137569/job/134455210533),
five sources were checksum-verified before vudc_transfer.c exhausted all three
attempts with HTTP 429. vudc_main.c had already required three attempts. The
17.144-second acquisition receipt and original failure artifact are retained.
This is a source transport failure before native module qualification.

The candidate uses a separate cache directory and an exact manifest-derived
GitHub Actions key. The official actions/cache v5 commit
caa296126883cff596d87d8935842f9db880ef25 was verified through its commit and action
metadata; it uses Node 24 and saves only on job success. There are no broad
restore prefixes. Cache contents are public pinned source bytes, with no modules,
credentials, logs or runtime state. The qualifier still starts from a fresh build
root and verifies every restored file, including checksum/size and linked-entry
refusal. Source manifest, patches, retry budgets and kernel admission controls
are unchanged.

Two new child-process controls exercise the real build dispatch boundary and
real source verifier against synthetic public bytes. Three independent build
roots read the same verified external cache with network forbidden. A restored
corrupt entry fails with an acquisition receipt and is neither overwritten nor
replaced through a network fallback. These are source-cache component controls;
the synthetic child does not compile a module or claim native success.

All eight source-acquisition controls and eight native-admission/child-boundary
controls pass locally. Workflow YAML and the explicit cache path/key were checked.
The [manifest](source-manifest.json) binds code, tests and retained receipts.
Hosted native USB qualification and actual Actions cache reuse remain pending.
A cold cache can still meet upstream HTTP 429, and a cache hit cannot replace the
negative/positive kernel controls. Audit #429 and all release gates stay required.
