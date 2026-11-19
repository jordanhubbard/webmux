# Native application gate and hosted job restriction

The unchanged application gate passes on the private Windows AMD64 target with
the clean d4b3e45 application image. It fails on hosted Windows at that same
source: the actual bootstrap reports an immediate job that disallows breakaway,
and owner process creation returns Windows error 5. The hosted component
recovery tests also skip after their breakaway probe is denied. See the
[source-bound receipt](native-application.json) and
[failed hosted run](https://github.com/jordanhubbard/webmux/actions/runs/44463372933).
No launch flag, job limit, retry or protection setting was changed.

The private run uses installed Microsoft Edge, Node26 and a fresh prebuilt
application on loopback, with private configuration and a fresh fixture home.
The test body is byte-identical to the hosted failing test. It requires fresh
TTY child output, matching original owner selection, authenticated Stop,
child exit before its independent timer, and an identical duplicate Stop.
One test passes in 8.184 seconds with no skips or retries. These are shell and
application checks, distinct from authenticated coding-conversation recovery.

After fixture cleanup there are zero saved sessions, no Restart Manager holders
for the application/helper images, and a free loopback port. The controller is
then terminated; no provider is manually killed. The provider cache directory
is empty and has no identity record. Raw logs, attachments, driver code,
source bindings, resource probes and failed attempts remain retained privately.

Normal browser discovery now covers the 36 fixture files. The positive native
gate has a separate configuration; discovery on Windows confirms it selects
exactly that gate. Its test body, deadlines and assertions are unchanged. The
manual Windows qualification workflow still invokes it explicitly and fails
when the selected hosted runner forbids independent providers. No fixture-only
green result establishes native provider acceptance.

On a Windows checkout with the normal build/browser prerequisites, run:

```powershell
cd webmux
npm run test:e2e -- --config=playwright.native-windows.config.ts --trace retain-on-failure
```

An installed Chromium browser may be selected with the existing
`PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` setting. Run on a host whose outer jobs
permit independent providers. The native configuration rejects non-Windows
hosts; a restrictive Windows runner fails the gate rather than skipping it.
The recorded private receipt used a prebuilt application and an isolated
Playwright configuration to avoid installing build/browser tooling there.

The new configuration passes all five strict TypeScript projects. Corrected
normal discovery has no import errors; the first direct discovery omitted the
normal runner's NODE_PATH and its failure is retained. Review, exact-head CI,
integration and broader Windows/release acceptance remain open.
