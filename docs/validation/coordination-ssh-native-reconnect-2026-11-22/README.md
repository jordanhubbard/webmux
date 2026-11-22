# Persistent native Claude across SSH reconnect

This receipt binds the original branch source. See the subsequent
[corrected-stack integration](../coordination-ssh-reconnect-integration-2026-11-22/README.md)
for the stronger completed-turn process observation and project-cache fix.

Issue [#630](https://github.com/jordanhubbard/webmux/issues/630) remains open.
This extends [ordinary SSH native adoption](../coordination-ssh-agent-2026-11-22/README.md)
with two turns in one live Claude Code 2.1.319 conversation on Linux ARM64,
controlled by a native macOS ARM64 WebMux source.

The fixture launches the ordinary shell alias once. A fixture-owned producer
holds its native stream-json stdin open after the first task. Only after that
turn completes does the source reconnect the WebMux SSH session. It then sends
the second task through the same stdin stream. No new CLI invocation or resume
selector is supplied, and no coordination text is injected into a terminal
prompt. Automatic MCP bootstrap and the existing private locator remain the
production paths. The explicit two-tool native approval profile remains in use;
other approval policies are not qualified by this result.

The final harness checks that reconnect replaces the private SSH control
connection, the old SSH master terminates before further native work, and the
exact provider pane, native-launch wrapper, actual Claude process and stdio MCP
child survive. Native process identities include Linux process start times to
exclude PID reuse. The same MCP process must then return a fresh scoped view
and record the second task. Both source ledger observations are retained.

The independent [auditor](../../../scripts/audit-coordination-ssh-agent.py)
requires actual paired tool results, complete subject and verified remote project,
ordered view/work calls with distinct request IDs, the two source ledger task
observations, unchanged process identities, a new channel with its predecessor
stopped, one conversation ID and two successful native turns. Both turns must
report their session-specific dynamic MCP server connected. Only the two scoped
tools and native tool discovery are permitted in the transcript.

The final run passes in 20.39 seconds with five actual tool calls across the
two turns. Thirteen altered-evidence controls all fail the audit, including changed
CLI/MCP/wrapper identities, a surviving old master, unchanged control connection,
a replaced pane, a different conversation, missing second-call results, false
success, repeated request ID, calls outside the second turn and a lost task. The complete Node 24 Go suite,
HTTP API vet and Python compilation checks pass.

## Failures and evidence boundaries

The first two attempts fail the original process-identity gate before reconnect.
A snapshot of the fixture's owned process tree proves that the production native
launch wrapper and actual Claude both use the process name `claude`. The corrected
probe identifies the actual native process as the direct parent of its scoped
stdio MCP child, and retains the wrapper separately. Neither failure proves a
reconnect defect; neither is counted as a passing reconnect run.

Run 3 passes the native test, but its initial audit fails an incorrect assumption
that stream input emits only one `init`. The raw transcript shows one init per
turn, with the same conversation ID. The corrected audit requires both init
records to share that ID and report the scoped server connected, alongside the
independent process continuity checks. The original failed audit is retained.
Runs 3 and 4 pass those checks, before the final old-master termination gate was
added. Their evidence does not establish that stronger gate.

The [receipt](receipt.json) separates the final native run from earlier revisions,
retains source/executable/evidence hashes and negative controls, and keeps raw
native conversations private under ignored `_build`. No installed service,
existing agent or account configuration is restarted/replaced. Authentication is
reused in place without copying credentials. Fixture-owned tmux namespaces and
socket caches are cleaned up; retained evidence is preserved.

Run `TestCoordinationSSHNativeBootstrap` with explicit owned host/root/helper/output
paths plus `WEBMUX_SSH_COORD_NATIVE_CLAUDE=1`, `WEBMUX_SSH_COORD_RECONNECT=1`
and optionally `WEBMUX_SSH_COORD_FIRST_USE=1`. Then audit:

```bash
python3 scripts/audit-coordination-ssh-agent.py "$fixture_output" --reconnect --output "$audit_output"
```

This qualifies a planned SSH reconnect between completed turns, with an existing
native CLI and MCP process. It does not qualify controller restart, uncertain
in-flight delivery, unexpected tunnel loss, durable claims/messages/reservations
through the complete lifecycle, managed/existing-agent onboarding, password SSH,
Mosh, Windows or the discovered/enrolled two-host workflow. Those #630/#631 gates
remain open. No roadmap checkbox is completed here.
