# Rig private tmux fixture — 2026-11-17

This addresses #431 under #194/#318. tmux 3.8 changed the default global mouse
option to on ([upstream CHANGES](https://raw.githubusercontent.com/tmux/tmux/3.8/CHANGES)).
A fresh, isolated native server started with `-f /dev/null` returns `on` before
any WebMux or Rig operation. The browser fixture likewise already has global
mouse on immediately after creating its unrelated sentinel, before starting Rig.

The fixture now writes a private tmux configuration with global mouse off and
uses it when creating the private server. Existing assertions remain unchanged:
managed sessions must enable mouse, global mouse must remain off, and the
unrelated sentinel must remain off. Production terminal behavior is unchanged.

The dashboard failure was a consequence of the first failure. Playwright runs
the worker's `afterAll` hook when replacing a failed worker; that hook stops the
shared fixture daemon. The next worker therefore opens the dashboard after the
daemon state file has been removed and displays its default stopped endpoint.
With the mouse fixture corrected, the dashboard enters live TOPOLOGY, opens
SYSTEM HEALTH, reuses its viewer on reload and detaches without stopping Rig.
No automatic daemon start or changed endpoint assertion was added.

Against production source `8f5d322`, the unchanged eight-case browser suite now
passes on macOS ARM64 with Node24.21.0, published Rig0.6.14 (`df1c1517`), tmux3.8
and Chrome157.0.8103.79. Strict TypeScript checks pass. See
[native-results.json](native-results.json) for source and private-log hashes.
All services, tmux sockets and homes are fixture-owned; user services and
configuration were preserved. This qualifies synthetic provider enrollment,
messaging and dashboard behavior, not authenticated coding-provider workloads.

A combined run with PR #432 passes all eight older controls, then the ninth
external-seat case hits the shared-address request budget in its browser UI.
Its focused native control previously passed. The additional case needs a
separate fixture lifecycle under #430; this run does not establish a green
nine-case suite. Production rate limits were preserved.

The dependency audit tracked in [#429](https://github.com/jordanhubbard/webmux/issues/429)
continues to block merge and release. The broader #194/#318 roadmap remains open.
