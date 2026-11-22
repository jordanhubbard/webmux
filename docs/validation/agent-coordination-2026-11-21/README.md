# Native ordinary-agent coordination startup

Qualified on macOS ARM64, 2026-11-21 local time. The [receipt](receipt.json)
binds final source, the native production helper, retained local evidence and
compiled Windows/Linux artifacts. Scope: automatic native MCP startup and
same-node project discovery, as part of [#630](https://github.com/jordanhubbard/webmux/issues/630).

For each installed native CLI, the opt-in Go test starts a real broker-owned PTY
with the production `--browser-shell` helper. Its normal shell bootstrap installs
the execution-host aliases. The test enters a plain `codex` or `claude` task
command, with no browser viewer, special provider form, MCP flags, custom startup
prompt or manual credential extraction. Transcript recording is disabled. The
task asks for collaboration context while forbidding edits, commands and
additional agents. Coordination guidance comes from the automatically attached
native MCP server. Native account/configuration homes remain in place.

Both Codex 0.171.0 and Claude 2.1.327 call `webmux_coordination_view`. Acceptance
inspects actual native tool-result events tied to this session, rather than the
model's prose. Each result has the expected contract, `node-local` scope,
`verified-git` project status and three same-account members across two worktrees;
a foreign account is omitted. The fixture peers are controlled ordinary PTY
children. These are two independent single-agent runs, not a simultaneous
multi-agent claim/handoff/reservation test and not a cross-host workflow.

Codex 0.171.0 initially timed out opening its default gRPC code-mode host.
Disabling the whole host failed closed and is not a supported workaround.
Keeping the host enabled while disabling only its gRPC transport passed. The
version-specific adapter now supplies that transport choice automatically;
explicit native CLI transport selectors retain their choice. The final native
run contains no test-only transport flags. Sandbox, approval, model and credential
settings are not changed by the adapter. The test's Codex task explicitly selects
read-only/never, and Claude retains its existing account policy.

Controls also cover:

- MCP discovery, startup instructions, view and action tool schemas; busy denial,
  expiry/conflict semantics, private-access reload and invalid endpoint refusal.
- Signed minimal live routing with recording disabled; no command/argv or routing
  observation is saved as broker recovery history. Verified project bindings
  remain durable coordination state.
- New PTYs discard outer tmux routing; durable panes override coordination
  locations from a shared tmux server. The client rejects mismatched session
  access files. A cold agent request resolves its same-account peers before the
  maintenance tick.
- Restricted/explicit MCP modes and unqualified native versions preserve their
  original invocation with a visible status. Original launch arguments remain
  intact, and bearer/recovery keys never enter the MCP definition's argv.
- The production executable's ordinary-child CLI/MCP exchange completes before
  any browser viewer opens, including a real legacy MCP initialize/tools handshake.
  The official SDK client controls exercise its negotiated protocol as well.

Final checks include the complete Node 24 Go suite with package concurrency four,
Go vet, selected server/recovery/session/terminal race controls, complete scoped
client/MCP race suites, strict TypeScript and the production contract. Windows
ARM64/Linux ARM64 production binaries and Windows ARM64 client/MCP tests compile;
this is not native Windows/Linux runtime acceptance.

The first full-suite run hit an unchanged 300ms Agent Deck fixture startup race,
now tracked in [#636](https://github.com/jordanhubbard/webmux/issues/636). Five
focused repetitions and subsequent full suites passed; its fixture is unchanged
here. Failed native experiments and their output hashes remain distinct from
admitted passing runs in the receipt.

Remaining: cross-node membership/delivery/authority, supported live notification
push, already running agents/older helpers and absolute-path bypass onboarding,
managed/SSH execution hosts, explicit project mappings, additional versions and
platform custody/sandbox/distribution acceptance, simultaneous real-agent overlap,
acknowledged handoff and shared reservation workflows, and full discovered
multi-host restart/reconnect acceptance. Roadmap boxes and #630/#631 stay open.
