# Production controller restart integration review

This integrates [#650](https://github.com/jordanhubbard/webmux/pull/650) onto the
corrected SSH reconnect/project-cache parent. Production sources are unchanged
from `0d332c2443d135775c4d30f3e3f7c4a6bd85436c`; the shared SSH setup script is
byte-identical after extraction into a test constant. The parent's full Node 24
Go, core/channel/session/API race, vet, TypeScript and production contract evidence
therefore retains its original production scope. Current HTTP API vet, race test
compilation, Python compilation and the fresh native run additionally exercise
the integrated fixtures and auditors.

The fixture launches two separate production-source WebMux processes against one
private retained controller home, bootstraps its own account through the public
API, and starts one ordinary SSH session. The original controller exits after an
interrupt before its replacement starts. One real Claude Code 2.1.319 conversation
retains its native launcher, actual Claude process, stdio MCP child and provider
pane. The fixture now holds the original input stream open until after both
native turns complete, and observes all process IDs/start times and the pane
again after the completed post-restart turn. Ordinary alias provisioning and the
explicit two-tool native approval profile remain in use.

A fresh macOS ARM64 to Linux ARM64 run passes in 26.68 seconds with both the source
production executable and test driver instrumented for races. The Linux production
helper's SHA-256 is rechecked on the worker before use. Before any new native
instruction, restore preserves the complete subject, task, original file claim ID
and exact expiry, and history epoch, with newly verified repository context. The
native agent then reads that retained state, updates its task and releases the
original claim without creating another. Seven actual tool calls complete in
one conversation with two successful turns. Process and pane identities still
match after the second turn completes.

The independent audit requires actual paired tool results and verifies project
identity in the restored/final source observations and the post-restart native
view. Both turns report the session-specific dynamic MCP server connected.
Twenty-three altered-evidence controls reject changed controller/native/pane
identities, missing completed-turn evidence, boolean process IDs, changed history,
lost tasks/projects, replaced or extended claims, missing native results,
changed conversations, a false success, static server configuration and an
unreleased claim. They edit private receipt copies and execute no native tools.

The initial current-stack attempt fails its unchanged 120-second observation gate:
the native agent records `exactly native SSH restart qualification`, including the
instruction word. The failed test, native transcript, last view and original
fixture source remain retained. Quoting the task and request-ID values explicitly
corrects that ambiguity. No assertion, deadline or native approval policy is
relaxed, and the failed attempt is not counted as a pass.

The [receipt](receipt.json) binds sources, executables, private evidence and
validation outcomes. Raw conversations, fixture credentials and retained homes
stay under ignored `_build`. Native authentication is reused in place, without
credential copies. Only fixture-owned controllers, tmux namespaces, socket caches
and SSH masters are operated. Installed services and unrelated providers remain
untouched.

This qualifies graceful controller restart between completed native turns for a
task and advisory file claim. SIGKILL, interrupted/uncertain delivery, full native
message/receipt/reservation continuity, unexpected tunnel loss, managed/existing
agents, password SSH, Mosh/Windows and discovered/enrolled two-real-controller
acceptance remain open under [#630](https://github.com/jordanhubbard/webmux/issues/630)
and [#631](https://github.com/jordanhubbard/webmux/issues/631). Hosted checks and main
delivery remain separate gates; no full roadmap checkbox is completed here.
