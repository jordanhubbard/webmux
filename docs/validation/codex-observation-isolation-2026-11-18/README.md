# Native observation startup isolation

Codex 0.169.0 policy inspection now uses two private processes. The first reads
configuration layers and the native account, without creating a thread. Native
workspace routing needs network access even with token refresh disabled. That
process exits before a second process reads the same configuration and creates
an ephemeral thread under the macOS OS guard. Configuration changes between
stages refuse before thread creation. Disabled project layers still refuse
without granting project trust, and neither stage submits a model turn.

Both processes disable hooks, plugins, apps and shell snapshots at startup. The
policy observer additionally denies networking and child-process creation. Its
initial exec is limited to the verified native executable. Denying process fork
also prevents an integration from starting a child using that same executable.
These restrictions apply to the disposable observer, not the actual coding
process or its saved native permissions. The account lookup may contact the
native service and update native state; this is not a filesystem-read-only
process guarantee. The public native-owned coding factory remains unactivated.

Enumerating configured MCP servers was insufficient: a server added after the
read could still start. The OS guard covers late stdio processes and HTTP
connections. Known servers are also disabled in the native thread request.
Native request overrides containing only `enabled:false` lose transports
supplied through CLI settings; copying the full normalized configuration fails
on native null/default fields. Retaining the minimal command/arguments or URL
with `enabled:false` handles both forms. Environment and HTTP headers remain
native and are not copied into the override or the exported launch plan.

The [source-bound receipt](receipt.json) records six actual native controls:
default full access, read-only and workspace-write, each with and without the
OS guard. A stdio process and HTTP connection added after configuration reading
start in each positive control and do not start under the guard. Native model,
project, permission policy, roots, approval and effort/tier fields agree between
paired controls, including workspace network=false and temporary-directory
exclusions. A same-executable child control verifies process-fork denial. Owner
configuration bytes remain unchanged, no turn is submitted, and owned native
process cleanup passes.

The maintained qualifier also uses the existing authenticated account in place,
with synthetic stdio and HTTP MCP servers configured throughout. The full real
resolver succeeds, neither integration starts, and the native default
full-access/never policy is retained. No authentication file is copied. Run:

```bash
python3 scripts/qualify-codex-observation-isolation.py \
  --codex /absolute/path/to/codex --node /absolute/path/to/node24 \
  --authenticated-cwd /absolute/path/to/project --output /private/receipt.json
```

Omitting `--authenticated-cwd` runs only private unauthenticated homes. No login
or configuration trust is granted by inspection. Unsupported native versions,
selected profiles, named permission graphs and unqualified observation
platforms refuse explicitly. Linux needs equivalent enforced isolation under
[#523](https://github.com/jordanhubbard/webmux/issues/523) before its native owner-plan path can be qualified; it does not fall back unguarded.

The current-source authenticated creation composition also passes: kill only
the original creation helper before the environment handoff, inspect the
original operation, retain the original native node ID, then execute one fresh
coding challenge with the owner's environment and permissions. Duplicate
requests and topology replay retain both running member PIDs. Checked Stop ends
only the coding seat; sibling survival and private socket/daemon/tmux cleanup
pass. This does not establish restored-conversation or reboot recovery.

A rebuilt Go controller and actual Chrome repeat the public namespace test:
discard a successful native materialization reply, restart the controller,
retain the original project and operation, edit the form, then explicitly
complete the original request. Responses are 202/202/expected host-error 502.
Only after native confirmation does SSH probe the deliberately unavailable host;
no coding member or viewer starts. This private browser fixture uses loopback
and auth-none, not authenticated browser/coding acceptance.

Local gates pass under Node 24.21.0: 202 helpers, five strict TypeScript projects,
coordinator Go race/vet, Go controller build and whitespace. Earlier candidate
failures and unsupported-runtime invocations are excluded from positive evidence.
Public native-owned member creation/readiness/attachment and conversation
recovery remain open, as do canonical empty-pod selection, profiles/platforms,
review, dependency audit #429 and the full roadmap/release acceptance.

The [Linux observation checkpoint](../codex-linux-observation-2026-11-18/)
adds a pinned controller helper and repeats current-source native controls.
This receipt retains its historical module hashes and platform boundary.
