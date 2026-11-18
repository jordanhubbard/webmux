# Native Rig wheel history — 2026-11-18

Linux CI on #511 source `84f4ee1` expected `WM_SCROLL_OLD` after twenty wheel
events, while the real tmux display was at position60 in73 history lines. The
mouse path had entered copy mode; the fixed count had not exposed the marker.
The original test file is byte-identical in this checkpoint's parent `5763dfd`.

Continue actual wheel input until the marker becomes visible, within the same
five-second assertion deadline. Keep the original twenty events, native copy-mode
check, selected-session mouse-on/global-and-unrelated mouse-off controls, old/new
markers, workspace scroll position and120-second managed-test limit. No product
source, fixture input or native copy-mode navigation shortcut changes.

A real published Rig0.6.14(df1c1517)/tmux3.8/Go/Chrome control increases fixture
input to100lines. Original assertions fail after20events at95/202history. The
same control with the fix passes after38events at185/202, with the old marker
visible and workspace coordinates unchanged. The full remaining managed-agent
workflow also passes. Final source restores the ordinary40linefixture and
removes instrumentation; that uninstrumented managed case and all eight native
Rig browser tests pass on macOS ARM64. Strict E2E TypeScript, frontend build and
whitespace pass. These use controlled coding children, not authenticated native
coding conversations.

Both original-assertion failures remain retained. The first used a body-only
attachment that the list reporter did not write to disk; the second persisted
its numeric receipt explicitly. A separate wrong-cwd npm launch exited before
testing and is excluded from native qualification. All sources, outcomes and
hashes are preserved in the [receipt](receipt.json) and private `rig-wheel524/`.

The original Linux dashboard-startup failure is not established as repaired by
this change; the unchanged dashboard case passing locally has its own platform
scope. Fresh exact-head hosted gates, audit#429 and broader#438/#431/#452
acceptance remain required. Refs #524.
