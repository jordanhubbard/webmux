# Combined roadmap candidate

This candidate composes the reviewed-source checkpoints in [#452](https://github.com/jordanhubbard/webmux/issues/452):
[#428](https://github.com/jordanhubbard/webmux/pull/428) independent underline/snapshot profiles,
[#447](https://github.com/jordanhubbard/webmux/pull/447) Rig/Agents coexistence and observation with its eight input candidates,
[#449](https://github.com/jordanhubbard/webmux/pull/449) Agent Deck owning-profile routing, and
[#442](https://github.com/jordanhubbard/webmux/pull/442) Windows Chromium AppContainer installation access.
All four pinned heads are retained as ancestors. Git merged the inputs without
conflicts; the composed behavior was then qualified together. No runtime or
fixture overlay was used.

## Local combined-source checks

On native macOS ARM64, all 398 frontend tests in 41 files, 92 helper tests,
strict types, frontend lint/build, native application build and 22 Python
source-cache/sysfs/apt/admission controls pass. The Python checksum-refusal
traceback is an expected negative control, not native USB acceptance.

Full server race tests, Darwin/Windows AMD64 server vet, retained UV/VT race
suites and vet pass. Provenance checks retain 241 VT upstream files with
16 explicit patches and 104 UV files with three explicit patches. The VT
package race run takes 119.192 seconds; snapshot/generator packages also pass.
Windows vet is compilation/analysis, not native Windows execution.

Thirteen coordinator/browser cases pass against the real published Rig,
owned tmux/Node processes and Chrome, including active Rig/observed-terminal
coexistence, host identity, recovered-view identity, activity, notification
cancellation and profile-routed Agent Deck fixture actions. These are not
new authenticated-provider or remote native recovery claims. All 1,356 real
Chrome renderer/lifecycle checks pass. Their 1,353 three-stage comparison
receipts comprise 259 bare references and 1,094 references sharing the hyperlink
profile; three additional checks cover lifecycle behavior. Every reference keeps
the snapshot parser profile disabled. The shared hyperlink profile changes link
lifecycle methods, so those cases are not bare-renderer evidence. This local run
is not Windows ConPTY acceptance.

The source checkpoint, exact inputs, private log hashes and limits are in
[source-manifest.json](source-manifest.json) and [local-results.json](local-results.json).
The initial type check failed because the new worktree lacked its frontend
workspace dependency link. Both dependency paths were configured; final checks
pass with runtime/test source unchanged. The failed setup log is retained.

## Parent Windows repair evidence

At exact parent `89ee719`, native Windows diagnostics pass 46 ordinary fresh
first loads and four separate deliberate Stop controls. All traces complete
with end acknowledgment and zero loss/drops. All eight observed network-service
primary tokens are AppContainers, with zero executable-access denial/restart
reports. Native permission scope/refusal/idempotence controls pass; executable
bytes and other ACL state are preserved. Full parent Windows application,
reconnect stress, screenshot/native-browser checks and Packaging also pass.
All 18 parent outcomes are terminal: 16 successes, one expected release-attachment
skip and dependency-audit failure. See [parent-windows-sandbox.json](parent-windows-sandbox.json).

The browser package and permission helpers are byte-identical to that parent.
This establishes inherited component provenance; it does not qualify the entire
combined candidate on Windows. The unchanged native protected-token requirements
and all 50 browser records remain in combined CI.

## Exact combined-head Windows checks

The combined head `10b365c` now has its own passing Windows server/ConPTY and
protected first-navigation jobs. All 28 ConPTY cases execute, and all 1,384
renderer/lifecycle checks pass. The 1,381 three-stage comparison receipts include
259 bare and 1,122 shared-hyperlink references, with all initial/continued/second
attachment comparisons equal. All references exclude the snapshot parser
profile; only the bare group preserves every checked reference method.

Fresh protected navigation passes 46 ordinary loads plus four deliberate Stop
controls, with all 36 traces complete/lossless and eight observed AppContainer
network-service primary tokens. Zero executable-denial or service-restart reports
remain. See [combined-windows-results.json](combined-windows-results.json) for
exact job/artifact bindings. These results do not complete the overall roadmap,
the separate #450 diagnosis, or the unchanged dependency audit.

## Remaining gates

The remaining combined CI outcomes must be inspected; a passing Windows subset
does not establish the full hosted result. A
Windows non-race full-VT-package repetition retains the context of the earlier
[#450](https://github.com/jordanhubbard/webmux/issues/450) panic. A later pass is
not a demonstrated repair for that separate failure.

[#429](https://github.com/jordanhubbard/webmux/issues/429) still requires supported
security remediation and the unchanged audit gate. No audit suppression,
toolchain downgrade, unreviewed patch, navigation replay or protection bypass is
introduced. Native installed-service/ARM/recovery gaps in #318, conditional
lifecycle ownership in #438 and the opt-in bridge in #246 remain open. Enabling
both managers alone does not provide a bridge. No merge, release, authenticated
provider/service/reboot acceptance or overall roadmap completion is claimed.
