# Account removal revokes session coordination authority

Integration review of [#635](https://github.com/jordanhubbard/webmux/pull/635)
found that a retained ordinary terminal could continue using its previously
issued coordination capability after its WebMux account was deleted. The
original regression returns HTTP 200 where 401 is required. The original failed
result is retained; the foundation was not merged with that behavior.

Coordination now shares federation's current account policy: local mode requires
an existing account, and unauthenticated mode permits only `anonymous`. The agent
handler refuses removed owners before repository inspection, authoritative
membership marks their retained terminals inactive for coordination, and
maintenance/launch provisioning cannot renew their access. The terminal lifecycle
remains with its existing owner.

`TestCoordinationDeletedAccountLosesAuthorityWithoutStoppingTerminal` uses the
existing ordinary PTY fixture, deletes the non-admin account through the public
API, and verifies:

- Its previously valid scoped read and work action are refused with HTTP 401.
- Its retained terminal remains connected with the same logical identity.
- Maintenance removes its ledger authority, and a subsequent explicit lifecycle
  reconnect cannot renew it.
- The other account's scoped interface still works.

The corrected regression and scoped HTTP API race controls pass. The full Node 24
Go suite, vet, production executable HTTP/WebSocket/restart/coordination contract,
core/client race controls and strict TypeScript checks pass. The core and
TypeScript sources are unchanged by the account fix; those checks ran on the
combined integration tree before the fix, while full Go, scoped API race, vet and
production contract checks ran again afterward. The [receipt](receipt.json)
binds source and private logs, including the original failing regression.

This verifies account admission and retained ordinary-session behavior. It does
not establish native coding-agent, managed-provider, cross-node or full discovered
workflow acceptance. #630 remains open, and no roadmap checkbox is completed.
