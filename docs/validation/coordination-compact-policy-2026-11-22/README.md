# Preserve compact native Codex configuration choices

Review of [#637](https://github.com/jordanhubbard/webmux/pull/637) found that
Codex 0.171.0 accepts compact `-cKEY=VALUE` options, while the coordination
adapter recognized only spaced `-c KEY=VALUE` and assigned `-c=KEY=VALUE`
spellings. A native `mcp list --json` probe with empty temporary `CODEX_HOME`
confirms that `-cmcp_servers.fixture.command="noop"` configures the owner's
server. This starts no model or MCP server and changes no existing settings.

Two regressions fail on the original adapter: a compact explicit MCP override
incorrectly receives added WebMux configuration, and a compact explicit
`features.code_mode_host_grpc=true` choice incorrectly receives the adapter's
compatibility transport override. Both original failed results are retained.

The adapter now normalizes only its inspection of Codex's compact `-c` spelling.
The original native arguments remain intact. Explicit MCP configuration returns
`mcp-policy-preserved`; an explicit transport selector prevents the compatibility
prefix while retaining normal scoped MCP attachment. Claude's argument spelling
is unaffected. Unknown versions and other native restriction controls retain
their existing behavior.

The fixed regressions and opt-in native parser checks pass. The native compact
case lists exactly the owner's `fixture` server and no added WebMux server; the
ordinary automatic MCP parser control still passes. These parser checks use an
empty temporary configuration and do not invoke a model or native agent workflow.
The [receipt](receipt.json) binds source and private before/after logs.

The updated #637 integration also retains #635's account-removal correction and
its regression. Full Node 24 Go, client/MCP race and scoped API race controls, vet, strict TypeScript and production CLI/MCP server
contract checks pass. Hosted checks must pass on the updated head before merge.
Earlier real native agent acceptance and its rejected experiments remain in
[the native bootstrap receipt](../agent-coordination-2026-11-21/README.md).

This is a native option-preservation fix, not new acceptance for other CLI
versions, explicit gRPC operation, managed/SSH onboarding or the complete
cross-node/discovered workflow. #630 remains open; no roadmap checkbox is
completed here.
