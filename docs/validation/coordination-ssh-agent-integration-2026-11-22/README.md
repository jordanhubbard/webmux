# Native SSH agent integration review

This [source-bound receipt](receipt.json) integrates
[#648](https://github.com/jordanhubbard/webmux/pull/648) with corrected
[#647](https://github.com/jordanhubbard/webmux/pull/647) for
[#630](https://github.com/jordanhubbard/webmux/issues/630).
Production sources are unchanged from the qualified SSH-channel parent. The
native-only fixture retains the live account-removal control and refuses mixing
it with the separate native adoption mode.

Two fresh current-source macOS ARM64 to Linux ARM64 runs passed with remote
Claude Code 2.1.319, a race-instrumented controller and a SHA-256-verified Linux
production helper. Ordinary SSH bootstrap and the normal shell alias provision
the dynamic MCP interface; no MCP definition or capability is handed to Claude
by the task. Native authentication is reused in place without copying credentials
or modifying native account policy. The task selects only its two scoped
coordination tools; other native policies remain authoritative.

Each independent audit requires actual native events: the dynamic server connects,
a verified view for the complete expected SSH subject precedes the work call,
that work call returns success, and the source ledger confirms the exact task for
that subject. Each run made three calls: discovery, view and work. All calls
completed and the native process reported success; capability prefixes appear in
none of the native transcript, stderr or terminal output. Owned-master exit and
private provider/cache cleanup remain part of the native fixture.

Six controls alter only private copies of the first run's audit inputs. Wrong
subject, wrong task, missing view result, missing action result, an unapproved tool
and a false success flag each fail their specific intended requirement. No
unapproved tool is executed by these offline controls. The original inputs and
prior receipt remain intact.

The current full Node 24 Go suite and vet pass. Core/channel/API race, TypeScript,
production contract and Windows compilation evidence from #647 applies to its
unchanged production components; the fresh native runs additionally instrument
the current controller and fixture for races. Raw conversation content stays in
private artifacts; public evidence records hashes and outcomes.

This establishes one remote native agent's adoption of the ordinary SSH channel
under the explicit task-selected tool profile. Native reconnect, complete
production-controller restart, managed/existing-agent participation, password,
Mosh/Windows and discovered two-controller workflow acceptance remain separate.
Hosted checks and parent delivery remain merge gates. Full roadmap acceptance
stays open.
