# Durable native topology and original-request inspection

Rig 0.7.18 new-project creation now allocates an empty native topology through
its supported `rig.materialize` operation before probing a member host or
starting an agent. The durable journal fixes the original operation UUID,
canonical body, controller-local root and namespace cwd. A lost reply returns
HTTP 202 with typed project uncertainty, before any viewer is allocated.
Ordinary duplicates do not post again. Explicit inspection can finish the
original request if it never arrived and otherwise replays its exact native
operation. Missing or changed original artifacts refuse replacement allocation.

The dialog retains the original submitted project, agent, host, workspace and
runtime. “Check and complete original request” uses those values even after
form edits. Dismissing the notice sends nothing. A stopped empty native project
is a valid enrollment and is reused by ID. Older cached adopted projects also
retain their original ID. Namespace allocation uses the controller's local cwd;
it does not replace the independent SSH member workspace.

The [source-bound receipt](receipt.json) records actual Chrome, Go controller
and Rig qualification. A test-local transport shim forwards the real native
materialization request, consumes its successful response, then deliberately
loses that response. The browser observes HTTP 202, one native project, zero
members/viewers and no SSH probe. After graceful controller restart, another
ordinary submit retains that original project without a materialization POST.
Editing the form and explicitly completing the request replays the original
UUID/body/headers. Only then does actual SSH probe the deliberately unavailable
host and fail. The result remains one project and zero coding seats/viewers.
The three public responses are 202, 202 and the expected host-error 502.
This fixture uses a private loopback controller with authentication disabled;
it does not establish authenticated browser or coding-session recovery.

A separate composed fixture uses authenticated native Codex 0.169.0 with the
same current topology/journal/factory modules. It kills only the original
creation helper after enrollment and before environment handoff. Explicit
inspection retains the native node ID, reconstructs the private once-only
handoff and starts one fresh coding process. Actual correlated tools verify
owner environment and default full-access policy. Duplicates and topology
replay retain both the coding PID and a running sibling. Checked Stop ends
only the coding seat. No arbitrary fixture value persists in durable state;
private socket, daemon and tmux cleanup pass. This is creation-helper recovery
before the first conversation, not restoration of an existing conversation.

Local checks pass: 199 helper tests, 420 frontend tests, five strict TypeScript
projects, 272 installed lock identities, frontend lint/build, Go build, HTTP API
and coordinator race/vet, and whitespace. The first browser harness attempt
referenced the wrong asset directory and is excluded from positive evidence.

Public coding launch still uses the existing adopted `attach --self` path.
The native-owned Codex factory remains internal: concurrent MCP startup
observation, public member uncertainty/readiness/attachment and recovery with a
fresh environment handoff remain incomplete. Empty existing-project selection
still needs canonical pod metadata. No new native Stop authority is granted to
adopted seats. Selected profiles, named permission graphs, Windows/native
platform acceptance, review, dependency audit #429 and full release gates remain
open. Earlier receipts retain their historical hashes; this receipt supersedes
the environment checkpoint only for the current source composition above.

The later [observation isolation checkpoint](../codex-observation-isolation-2026-11-18/)
binds the changed owner resolver and repeats native creation and public namespace
qualification with the rebuilt controller. This receipt retains its historical hashes.
