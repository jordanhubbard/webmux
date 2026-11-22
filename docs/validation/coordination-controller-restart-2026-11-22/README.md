# Native SSH task and claim across production controller restart

This receipt binds the original branch source. See the subsequent
[corrected-stack integration](../coordination-controller-restart-integration-2026-11-22/README.md)
for process/pane observations after the completed second turn and stronger project controls.

Issue [#630](https://github.com/jordanhubbard/webmux/issues/630) remains open.
The final native macOS ARM64 to Linux ARM64 run passes in 20.71 seconds.
Unlike the earlier broker reconnect fixture, this starts two separate production
WebMux executable processes against the same private retained controller home.
The first exits after an interrupt; the second performs normal startup restore.
No installed service or existing user provider is restarted.

One remote Claude Code 2.1.319 conversation remains alive with its original native
launch wrapper, actual Claude process, stdio MCP child and tmux pane. Process
identities include Linux start times. The fixture starts the ordinary SSH session
through the public HTTP API, creates a fixture account through bootstrap, and
supplies one normal task command through its authenticated terminal WebSocket.
The native alias automatically supplies MCP; there is no hand-written MCP
configuration or capability extraction. A private stream-input producer holds
Claude's input open between turns. The explicit two-tool native approval profile
is the same as in the [SSH adoption qualification](../coordination-ssh-agent-2026-11-22/README.md).

The first native turn reads context, registers its task and claims
`restart-owned.txt` for 600 seconds. After that turn completes, the controller
stops and restarts. Before any new native instruction, its public coordination
view must retain the complete subject, task, original claim ID and exact expiry,
and history epoch, after verifying the restored provider's project. The second
turn reads that retained claim, updates the task and releases the original claim
without creating a replacement. Seven actual native tool calls complete.

The independent [auditor](../../../scripts/audit-coordination-controller-restart.py)
requires actual paired native tool results, process and conversation continuity,
two successful native turns, ordered work on both controllers, stable request
IDs, retained claim/history/task evidence, the original claim's release and the
source's final task/empty-claim observation. Only scoped tools and native tool
discovery may run. Capabilities must not appear in native output, stderr or the
retained terminal events. Thirteen altered-evidence controls reject changed
controllers/processes/pane/history, lost tasks, replaced or extended claims,
missing native results, changed conversations and an unreleased claim.

## Original failures and scope

Attempt 1 fails before starting a controller because the remote qualification
cache, including the staged helper, had disappeared. The host/account were
verified and the retained local helper was restaged with its SHA-256 checked.
Attempt 2 fails the public API rate limit while polling at ten requests per
second. Polling is now once per second; no production limit is changed.
Attempt 3 reaches its original 120-second observation deadline: the fixture
incorrectly waits for its scratch repository before sending the command that
enters it. A retained snapshot shows the configured channel and unavailable repository
verification; the source trace shows the repository-changing command is sent
only after that gate. The corrected initial gate requires a live
configured channel; subsequent task/claim gates still require verified Git
context. Neither deadline is increased. All three failures remain failures.

The [receipt](receipt.json) binds source, production executables and private logs,
including those original failures. Raw native conversations, fixture credentials,
controller homes and logs remain private under ignored `_build`. The complete
Node 24 Go suite, HTTP API vet, Python compilation, native test and independent
audit pass.

Run `TestCoordinationProductionControllerRestart` with
`WEBMUX_TEST_PRODUCTION_COORDINATION_RESTART=1`, explicit SSH host/user/root/helper,
a fresh local output directory in `WEBMUX_SSH_COORD_DIR`, and a built production
source executable in `WEBMUX_SSH_COORD_SOURCE_HELPER`. Audit afterward:

```bash
python3 scripts/audit-coordination-controller-restart.py "$fixture_output" --output "$audit_output"
```

This qualifies graceful production controller restart between completed native
turns for a task and advisory file claim. It does not qualify SIGKILL, interrupted
or uncertain delivery, message/receipt and reservation continuity across the full
native lifecycle, unexpected tunnel loss, managed/existing-agent onboarding,
password SSH, Mosh, Windows or the discovered/enrolled complete two-host workflow.
Those roadmap requirements remain open; no checkbox is completed here.
