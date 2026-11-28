# Linux native companion sandbox preflight (#784)

The E2E outer runner now checks the actual selected companion before Playwright
discovery or server-fixture allocation. Selection precedence is unchanged and
shared with the server fixture. Successful Linux admission freezes that executable
for child runs and records the browser version. The existing driver readiness
check remains separate.

The control launches full Chromium through Playwright with `chromiumSandbox: true`
and reuses the existing bounded readiness probe: render a data page in a private
context, close it, confirm the browser survives, then repeat in a fresh context.
Launch and navigation deadlines are unchanged. Playwright closes the owned browser
and contexts. No host sandbox policy or production companion flags changed.

Native Linux x86_64 controls on 2026-11-27:

| Selected companion | Outer runner `--list` result |
| --- | --- |
| Managed full Chromium | Refused before discovery: host sandbox admission failed |
| Explicit installed Google Chrome 158.0.8127.36 | Passed sandbox-enabled readiness and listed tests |
| Explicit nonexistent executable | Refused before discovery, without fallback |

This is sandbox/CDP readiness evidence. It does not prove native application
profile/lease handling, mobile/accessibility acceptance, provider authentication,
or a new macOS/Windows runtime result. Existing application assertions still run
unchanged in the full suite. Non-Linux runners retain their previous admission
behavior. An explicit user selection is never replaced after refusal.

`receipt.json` binds maintained sources, native-control log hashes and the full
local quality gates. Private logs retain the actual Chromium sandbox refusal.
