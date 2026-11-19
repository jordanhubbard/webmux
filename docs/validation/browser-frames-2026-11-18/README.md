# Browser frame controls — 2026-11-18

Issue: [#507](https://github.com/jordanhubbard/webmux/issues/507), continuing
[#110](https://github.com/jordanhubbard/webmux/issues/110). This change is stacked
on `473da1123a96dc10f9740ef57361c2b3571cf812` (#503), which includes canonical
roadmap candidate #500. It does not replace that candidate or authorize a release.

The selected tab's accessibility snapshot now includes supported local and
out-of-process frame documents. Each opaque control reference records its actual
frame/document/loader ancestry. Cross-process documents require a native frame
owner in a proven parent document; URL, name and browser-context matches do not
establish ownership. Native nodes resolve in an isolated world pinned to the
expected document, with universal access disabled, so the final document check
and click/focus happen together even after same-origin node adoption.

Explicit discovery is limited to 32 documents and 32 temporary iframe target
attachments. Output still shares the existing 512-entry, 32-KiB name/role and
depth-32 limits across documents; omitted controls retain no public references.
Attachments use `WithTargetID` and explicit `WithDetachOnCancel`. Site isolation,
frame sandbox restrictions, the worker deadline, existing HTTP schema, owner
checks, controller leases and single-use references remain in effect.

## Local evidence

Actual Chrome **158.0.8127.36** used the production worker allocator. The native
test inspected its command line to refuse disabled sandbox/site-isolation flags.
`go test -race ./internal/browser -run '^(TestNativePageFrameControls|TestFrame.*)$' -count=1 -timeout=2m -json`
passed with `WEBMUX_REAL_BROWSER=1`: 25 native subtests plus both aggregate/frame
unit controls, no failures or skips. The native case took 10.14 seconds.

Coverage includes root, same-origin, cross-origin, nested local/cross-process,
return-to-parent-origin and closed-shadow controls; native focus followed by real
keyboard delivery; exactly-once activation; duplicate and replaced references;
child/ancestor navigation, removal and replacement; node adoption in both
directions; and cancellation before dispatch and after target attachment.

The dispatch-boundary controls mutate nodes **after** ancestry validation to
exercise the pinned-document check directly. A renderer-loss control observes
Chrome's `Inspector.targetCrashed` event before checking action refusal and root
page survival. A separate tab has an iframe with the **same URL and name** as an
owned frame: only one owned actionable control is exported. Reads/actions and
cancellation preserve the fixture's existing tab/frame target set.

Sandbox controls confirm that a frame without script permission cannot execute
its handler, and that a sandboxed frame with scripts cannot navigate the top
page. Editable values and unrelated frame text stay out of the snapshot.

The real Go/HTTP worker Playwright test
`browser-accessible-actions.spec.ts` passed (9.8-second case; 19.9-second run).
It covers lease denial before frame actions, single-use activation, native
same/cross/nested-frame typing, cross-origin confirmation with takeover/deferred
resize/input blocking, and the original root-dialog and closed-shadow controls.

Also passed: `go test -race ./...` (31 server packages), `go vet ./...`, the
frontend production build, and all five strict TypeScript projects. CI now runs
the native frame control with the race detector on Linux and retains its JSONL
alongside existing browser parity artifacts.

Retained local log SHA-256 values:

| Evidence | SHA-256 |
| --- | --- |
| Native frame and aggregate controls | `cf37f02b723fb0d227159495cc79a8b4c4f871cdb590f20514716971dfa3f5f6` |
| Root/cross-origin modal HTTP test | `989948f11dcd2e09189d3ad3680e91fe7f984fcfae16067f974162a3d3ba938f` |
| Full server race suite | `38b878fc163d6b6d06c6e9bc46826e1997b72ac3d96ef5215273e5620ff831f2` |

These are local native/browser fixture results. Hosted exact-head checks remain
separate. This does not establish assistive-technology, physical-mobile,
identity-provider, security-key, native SSO or client-certificate acceptance.
Those boundaries remain under #110. Audit #429 and the other release gates are
unchanged.
