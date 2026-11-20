# Current small-screen browser acceptance

The [receipt](receipt.json) completes #454's scoped small-screen requirement with
fresh native application/browser-worker execution. Broader #110 physical mobile
keyboard, iOS/Android/Safari, assistive technology, external identity-provider
and hardware-key qualification remains open, as do recovery and release gates.

The original pre-fix native log remains verified: source 10b365c gives the
320×568 address input only 95.6875 pixels and fails its 180-pixel assertion.
The integrated responsive layout gives the current portrait input 225 pixels,
with reachable navigation, control ownership, profile settings and Return to
terminal. No production behavior is changed by this follow-up.

## Current native execution

Clean source `2a17db11bc036bf2adc6344182a055ef2c1d70db` adds one maintained test;
its frontend and Go/server trees equal production `2bf7d7c`. Six files run with
fresh private controllers and tmux namespaces, each passing once with zero skip,
retry, flaky or unexpected results:

| Case | What it establishes |
| --- | --- |
| Original mobile companion | 320×568 portrait, 568×320 landscape and reduced-height 390×400; address width, discoverable profile settings, no horizontal overflow, reachable actual resized worker frame, once-per-touch native side effects and return to the same terminal. |
| New mobile page-text case | At all three sizes, actual worker accessible controls remain reachable with at least 44-pixel button height and no horizontal overflow. Touch activation dispatches once; focusing the real page field accepts initial and continued typing. Shift+Esc returns to the address bar, and Return to terminal preserves the original tile. |
| Original saved-profile case | Real saved cookie/profile persistence across worker replacement, viewer control exclusivity, refused foreign mutations and owned lifecycle cleanup. |
| Original interactive workspace | Native page navigation, dialogs, clipboard, files, tabs, touch/input and worker controls remain functional. |
| Original accessible actions | Actual live/stale references, identity/ownership refusal and once-only dispatch remain enforced, including native field and frame actions. |
| Existing pane layout | Docking/moving/resizing preserves the live page, controller and terminal layout. |

All four existing regression files remain byte-identical to production `2bf7d7c`.
The pane case and new
page-text case are additional evidence, not replacements for an omitted case.
The original 90-second mobile test, other native deadlines and production request
budgets remain intact. The privately named `parent-page-text` execution is a
successful current baseline; it is not an expected-failure or pre-fix result.
No new runtime defect was reproduced, so no speculative behavior change is made.

The current full frontend suite passes all 472 tests; all five strict TypeScript
projects, frontend lint and build pass. The new test is included in maintained
strict source coverage. Portrait, landscape and page-text landscape screenshots
are visually inspected; all eight generated screenshots are retained and hashed.

All six executions use the same independently hashed native application bytes.
The retained Mach-O build-data fields report the clean test source above; those
are producer observations, not independent VCS attestation. The binary and
screenshots are actual native macOS ARM64/Chrome artifacts. Real worker requests
are not replaced by protocol fixtures or mocked browser API responses.

Each completed fixture has no remaining executable holders before preservation;
the final executable also has none. The original browser/provider cleanup
assertions pass. Inactive private homes remain locally retained, and only
selected source, executable, results and diagnostics are archived; no native
account or external browser sign-in store is copied.

This proves browser-emulated touch and reduced geometry with real desktop-native
workers. It does not prove a physical software keyboard, mobile Safari, device
accessibility or external-provider authentication. No hosted workflow, installed
service/protection change, account login, release version/tag or publication is
performed. Whole-current-main release acceptance remains incomplete.
