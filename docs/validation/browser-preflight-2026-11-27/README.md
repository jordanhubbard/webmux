# Linux native companion sandbox preflight (#784)

For actual tests using the default application configuration, the E2E outer
runner checks the selected companion before Playwright discovery or server-fixture
allocation. Browser-free custom configurations and listing/help commands retain
their browser-free startup. Explicit selection of the default config still uses
the same companion guard. Selection precedence is unchanged and
shared with the server fixture. Successful Linux admission freezes that executable
for child runs and records the browser version. The existing driver readiness
check remains separate.

The control launches full Chromium through Playwright with `chromiumSandbox: true`
and reuses the existing bounded readiness probe: render a data page in a private
context, close it, confirm the browser survives, then repeat in a fresh context.
Launch and navigation deadlines are unchanged. Playwright closes the owned browser
and contexts. No host sandbox policy or production companion flags changed.

Native Linux x86_64 controls on 2026-11-27:

| Selected companion | Default application test / explicit readiness result |
| --- | --- |
| Managed full Chromium | Refused before discovery: host sandbox admission failed |
| Explicit installed Google Chrome 158.0.8127.36 | Passed the explicit sandbox-enabled readiness check |
| Explicit nonexistent executable | Refused before discovery, without fallback |

This is sandbox/CDP readiness evidence. It does not prove native application
profile/lease handling, mobile/accessibility acceptance, provider authentication,
or a new macOS/Windows runtime result. Existing application assertions still run
unchanged in the full suite. Non-Linux runners retain their previous admission
behavior. An explicit user selection is never replaced after refusal.

`receipt.json` binds maintained sources, native-control log hashes and the full
local quality gates. Private logs retain the actual Chromium sandbox refusal.

## Browser-free invocation regression and correction

The initial head `38e22c6d542d0abd26f61d1940f556bbe4ad2492` incorrectly applied
companion admission to every use of the generic runner. CI 46093411783 correctly
failed the existing receipt-preservation test: its custom Playwright fixture
needs neither browser nor application server, and the clean helper job had no
browser installed. The local run had an installed browser and masked this scope
mistake. The original failed CI and local reproduction remain retained.

Admission now belongs only to actual default-application test runs. The original
receipt test keeps every assertion/deadline and explicitly sets all browser paths
to absent locations. The complete helper suite also passes with all browser paths
absent. Default application runs still refuse a missing executable or host sandbox
failure before fixture startup, and the installed browser still passes sandboxed
render/reopen readiness. A default `--list` with absent browser paths succeeds
without launching an application fixture. Updated source/log hashes are bound in
the receipt; the earlier `--list` refusal was an initial-head result and is not a
promise for metadata-only commands in the corrected scope.
