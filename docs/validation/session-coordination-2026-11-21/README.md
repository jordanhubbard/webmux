# Durable node-local coordination qualification

Qualified on native macOS ARM64, 2026-11-21 local time. The
[receipt](receipt.json) binds source files, retained local logs and production
cross-build hashes. It covers the [node-local implementation](../../session-coordination.md),
not complete #630/#631 acceptance.

Passed controls:

- The real bbolt ledger survives reopen. A subprocess exits with code 73 after
  committing a handoff and during a subsequent uncommitted task transaction.
  Retry yields exactly one handoff and no uncommitted task change.
- Thirty-two competing reservation requests produce exactly one admission;
  fences survive reopen and reject stale renewals. A cached busy response never
  becomes a later admission. Clock rollback and missing authority metadata refuse
  new authority; a second ledger owner cannot take custody of the open database.
- Project/account isolation, linked worktree identity, claim overlap notices,
  handoff receipt ordering, expired admissions, request-ID payload reuse refusal,
  capability revocation and global policy controls pass deterministic tests.
- Real ordinary PTY test children inherit their own scoped CLI interface.
  Scoped HTTP admission rejects foreign-owner/project actions, root-API use,
  substitute browser credentials and deleted identities. A reconstructed
  controller refuses a retained capability when only saved connected flags exist.
- The production binary launches an ordinary Node child that invokes the real
  `webmux coordinate view` CLI before any browser WebSocket opens. Existing
  HTTP/WebSocket/restart contracts pass. Disabling coordination leaves its
  terminal connected. These children are test fixtures, not real coding agents.
- Unix private-file checks and loopback-only/no-proxy/no-redirect client controls
  pass. Planner tests strip inherited coordination credentials on Linux, macOS
  and case-insensitive Windows environment names; only supplied session locations
  are added, with no bearer in argv.

Final gates: full Node 24.22.2 `go test -count=1 ./...`, `go vet ./...`, core
race suite, scoped HTTP/terminal race controls, strict TypeScript checks and
`npm run test:contract`. Windows ARM64 and Linux ARM64 production binaries and
Windows ARM64 core/client test binaries cross-compile. Cross-compilation is not
native Windows/Linux runtime acceptance.

Native LLM startup guidance/adoption, managed/SSH execution-host onboarding,
provider inbox/sandbox reachability, cross-node messaging and authority, explicit
project mappings, Windows ACL custody, installed-package acceptance and the full
two-real-host discovery/agent workflow remain open. The source binds a logical
broker session incarnation, not a separately verified native process identity.
The roadmap checkboxes remain unchecked.
