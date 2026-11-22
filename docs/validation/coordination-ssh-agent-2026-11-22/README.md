# Native Claude adoption through ordinary SSH

Issue [#630](https://github.com/jordanhubbard/webmux/issues/630) remains open.
Two native macOS ARM64 to Linux ARM64 runs passed with remote Claude Code
2.1.319. The ordinary SSH broker session bootstraps its private channel and
the normal shell alias supplies dynamic MCP configuration. The fixture supplies
no hand-written MCP definition, locator contents or capability to Claude.

The task asks Claude to inspect its collaboration interface and register one
task. Its native invocation explicitly approves only the two session-specific
coordination tools using `--allowedTools`; this qualifies that permission
profile, not unattended approval under other native policies. Existing account
authentication is reused in place through `CLAUDE_CONFIG_DIR`. No credentials
are copied, CLI installed, service restarted or existing provider replaced.
Each run owns an isolated fixture repository, home, cache and tmux namespace.

The independent [auditor](../../../scripts/audit-coordination-ssh-agent.py)
checks actual paired native tool-use/tool-result events, not assistant prose:

- The session-specific dynamic MCP server connected.
- A successful view returned the complete expected subject and verified SSH
  execution-host repository before the work action.
- The native work call succeeded with a request ID and the exact task text;
  the source ledger independently recorded it for that subject.
- Only tool discovery and the two approved coordination tools ran, all calls
  completed, and Claude emitted a successful native result.
- No capability appeared in the native transcript, stderr or terminal output.

Both runs made three tool calls: discovery, view and work. The test also checks
that source close terminates its owned private SSH master. The second run uses
the final harness source; the first predates improved failure-log retention.
Neither actual native attempt failed. Both passed their original 120-second
native task deadline in about twelve seconds. The original shell startup
deadline remains unchanged.

Six controls mutate private copies of the first transcript or expected evidence.
Wrong subject, wrong task, removed view result, removed action result, unexpected
`Bash`, and a false native success flag all fail the audit. The public
[receipt](receipt.json) retains their failure names and source/evidence hashes;
native conversation contents remain private under `_build`.

Run `TestCoordinationSSHNativeBootstrap` with the explicit SSH fixture variables
documented in its source and `WEBMUX_SSH_COORD_NATIVE_CLAUDE=1`. Use a new owned
remote root and local output directory for each run. Then run:

```bash
python3 scripts/audit-coordination-ssh-agent.py "$fixture_output" --output "$audit_output"
```

The full Node 24 Go suite and HTTP API vet check pass. This adds single-agent
native SSH adoption evidence to the [shell channel qualification](../coordination-ssh-channel-2026-11-22/README.md).
Native agent reconnect/controller restart, durable coordination continuity,
managed/existing-agent onboarding, password startup, Mosh, Windows and the
complete discovered two-host workflow still require acceptance. No roadmap
checkbox is completed here.
