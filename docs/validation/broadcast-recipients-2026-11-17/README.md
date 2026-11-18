# Stable explicit Type to All recipients

[#461](https://github.com/jordanhubbard/webmux/issues/461) corrects the recipient
selection under #194. The parent stores only exclusions of the sessions known
at selection time, so a newly registered terminal receives input after a
project/subset choice or **None**. Selection and mode refs also lag until an
effect, allowing the next route call in that event to use the preceding intent.

The context now stores selected IDs after an explicit choice. New registrations
stay unselected, current viewers can be included deliberately, and the same
selected ID retains its choice across sender reconnection. The initial default
continues to broadcast to registered terminals; choosing **All** explicitly
captures the current inventory. Manual choices also capture current recipients.
Checkboxes/tile controls use the same membership predicate as routing, and mode
and selection refs update before the next route call. Ordinary input stays at
its source. Help and the recipient strip explain the fixed selection.

The unchanged parent `4475544` fails seven of ten initial routing controls and
passes three. All ten control bodies remain unchanged in the final test file,
with two additional cases for an unlisted registered terminal and default-versus-
explicit-All behavior. The seven pre-existing context tests are retained verbatim;
the final file has nineteen passing controls. Full frontend qualification passes
410 tests in 41 files, strict types, lint and production build.

Five native macOS ARM64/Chrome browser cases pass: explicit recipients, existing
mode-query/editor keys, and mouse broadcast/restart/selection. A controlled Node
child requires an actual PTY and appends the bytes it receives to private files.
The original chosen child receives `xy`; a child opened after selection receives
only `y` after deliberate inclusion; the unselected peer receives only its own
`b` after broadcast is off. WebSocket input frames agree. These are host-side
input receipts, not an inference from terminal screen text. The test uses the
supported session API and production inventory-refresh event without reloading
the provider or replacing runtime source.

[Source/log bindings](source-manifest.json) retain the exact parent-negative
controls, final checks and native byte receipt. All seven original context controls are retained unchanged in the final suite.
The accepted full-suite log contains 410 passing tests; intermediate draft runs
are retained privately and are not substituted for that result.

Hosted exact-head qualification remains required. This is frontend routing and
controlled local PTY acceptance, not authenticated Rig/Agent Deck, remote-host,
Windows or lifecycle-ownership acceptance. #194/#318/#438/#246 remain separate;
#429 still blocks merge/release. No provider lifecycle, terminal key mapping,
backend authentication, dependency or release change is introduced.

Hosted head `7527664` passed packaging, browser startup diagnostics and six CI
jobs. Besides the known #429 audit failure, Windows failed the new test's exact
WebSocket-frame assertion after all three actual PTY byte checks passed. Its
default failure rendering shows identical expected/received strings, so it does
not identify the differing bytes. The test now compares the same exact bytes as
hex and persists both PTY and per-frame hex receipts in its failure path. It does
not filter or normalize traffic. Strict types and all five local native cases
pass again. The Windows cause remains unestablished until the diagnostic head
runs; this is not reported as a fixed Windows product failure.
