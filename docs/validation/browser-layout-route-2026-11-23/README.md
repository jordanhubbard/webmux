# Browser layout route lifecycle

Fix for #683. Native Windows CI at the pinned head in `receipt.json` failed
because the transient-notice route was removed while a polling frame handler
still awaited its real response. The subsequent fulfillment threw an unhandled
route error. Preserve the original failed log hash and native job link.

The test owns one interceptor. `page.unrouteAll({ behavior: 'wait' })` drains
running handlers before route removal, both after the notice clears and during
teardown. All geometry, input, page/control identity, single browser start/
navigation, reload and desktop layout assertions remain unchanged. Errors are
not ignored. This changes test lifecycle only.

Strict TypeScript, production build and the full local Go/Chrome browser-layout
workflow pass. Local Chrome uses an isolated Playwright profile. This result does
not establish native Windows success; fresh-head Windows CI remains required.
No installed runtime, permission, profile or NAS state changed.
