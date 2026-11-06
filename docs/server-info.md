# Server compatibility metadata

`GET /api/server-info` is a small, read-only compatibility observation. It does
not invoke provider discovery, enumerate sessions, return account/configuration
data, or authorize another operation. Public `/api/health` and
`/api/auth/status` retain their existing liveness and login-discovery responses.

The response identifies `webmux-server-info/v1` and the compiled
`webmux-coordination/v1` and `webmux-server-directory/v1` contracts. A compiled
contract is not runtime provider availability. Use the separately authorized
coordination inventory for enabled providers and their actual capabilities.
Unknown fields can be ignored, but an unsupported contract version must not
silently select a newer protocol.

## Existing authentication boundaries

In local authentication mode, use an existing account JWT. Administrators and
ordinary accounts can read this metadata; coordination still requires an
administrator, and every action retains its own authorization checks. Invalid,
expired or deleted-account tokens are refused by the existing middleware.

In trusted `auth:none` mode, the existing middleware permits anonymous access.
The response says `trusted-anonymous`, including for coordination, while account
administration is unavailable. This is not authenticated access. Supplying a
bearer string does not turn anonymous access into a validated account or grant.
A client must make its existing trusted-mode choice explicit.

Incoming federation grants remain restricted to existing one-owner terminal
routes. They do not authenticate this local-account endpoint or coordination.
No `/api/federation/server-info` route or connected-server metadata proxy is
added. In trusted mode this endpoint is accessible anonymously regardless of a
supplied grant string; that does not expand the grant's authority. Terminal
ownership is API isolation, not an operating-system sandbox.

## Self-reported build observations

The `build` object always contains nullable `version`, `revision` and `modified`
fields, a provenance label, and `attested:false`. A plain Go build with no
producer observations returns:

```json
{
  "version": null,
  "revision": null,
  "modified": null,
  "provenance": "unavailable",
  "attested": false
}
```

The standalone builder reads the validated `webmux/package.json` version and
optional source Git observations before copying into its private staging
directory. It rechecks those observations before compilation and embeds them
in the executable. An inherited `GIT_DIR`/`GIT_WORK_TREE` cannot select another
repository. Git observation failure or a source archive leaves unavailable Git
fields unknown; a package version alone does not imply a revision or clean
state. Missing package metadata in a module-only copy also stays unknown.
Malformed package versions are refused before compilation.

Available observations have `build-producer-self-report` provenance. They are
neither a signature nor a claim that every input byte equals a Git tree. A clean
Git observation is not artifact attestation, a released/qualified build, or
permission for a client to mutate anything. Do not derive missing facts from
the client checkout, server name, health response, installed filename or ambient
tag. Missing routes, unsupported schemas and unknown build values remain
explicitly unqualified compatibility until independently reviewed.

## Terminal and integration limits

Static limits describe the existing contracts: terminal messages acknowledge
queue admission rather than processing, and do not offer idempotent delivery.
An uncertain send must not be replayed automatically. Dynamic provider
registration and structured Codex conversation history/goals are not supplied
by these contracts. This endpoint does not add a plugin loader, bridge adapter,
authentication scope or conversation parity.

Responses use `Cache-Control: no-store`; observe the actual running server
instead of reusing stale compatibility metadata after an upgrade.
