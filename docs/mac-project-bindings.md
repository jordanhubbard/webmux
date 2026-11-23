# MAC observation binding contract v1

Implementation foundation for [#673](https://github.com/jordanhubbard/webmux/issues/673)
and the [project authority contract](mac-project-coordination.md). The Go package
[`internal/maccoord`](../webmux/server/internal/maccoord/bindings.go) parses and
resolves this versioned operator document. It has no file loader, environment
discovery, native credential-store access or dispatch path. A separate opt-in
read-route qualifier is described below. No product setting currently enables
this mapping or invokes that qualifier. The read adapter and its human/agent consumers
remain [#674](https://github.com/jordanhubbard/webmux/issues/674).

```json
{
  "schema": "webmux.mac.project-bindings.v1",
  "authorities": [
    {"id": "configured-fleet", "origin": "https://mac.example.test"}
  ],
  "bindings": [
    {
      "owner": "webmux-account",
      "repository_id": "exact-server-resolved-repository-identity",
      "repository": "example.test/team/project",
      "authority_id": "configured-fleet",
      "project_key": "exact-mac-project-key",
      "capabilities": ["project.observe.v1"]
    }
  ]
}
```

The values above are illustrative, not enrollment instructions or live fleet
configuration. Do not identify a project from its display name, local directory
or successful graph request. The repository ID includes the existing resolver's
canonical remote and root-commit witnesses; an explicit administrator mapping
is also supported. The resolver's account, ID, canonical repository and
`verified-git`/`explicit-map` provenance must all match the configured scope.
Session/browser input cannot supply its own verified observation.

MAC currently selects graph tasks by an authority-scoped `task.project` string.
`project_key` names that exact key; it is not a fabricated project UUID. An
operator-selected authority ID identifies a configuration entry, not a verified
remote principal. HTTPS origins are accepted; cleartext is limited to explicitly
configured loopback origins. Credentials in URLs, path/query/fragment routing,
invalid ports and unsupported schemes are refused. Later transport code still
needs normal TLS checks, fixed operations, redirect refusal, bounded responses
and actual upstream authentication/capability qualification.

There is exactly one binding per account/repository ID. Conflicting or duplicate
entries are refused, including otherwise identical duplicates. Unknown schema,
unknown fields, case aliases, duplicate JSON keys (including escaped equivalents), invalid
text, excessive nesting, oversized documents and unsupported capabilities are
refused. Credentials have no field in this contract. Errors do not echo policy
contents, other accounts, origins or supplied secrets.

Only `project.observe.v1` is requested by this schema. It does not grant that
operation against a live authority. Dispatch, participation, claims, leases,
resource fences, review and publication cannot be enabled by adding capability
names. No machine nickname is an authenticated worker identity. No session,
task attempt, lease, source revision or admission outcome is inferred from a
project selection. The later observation and participant contracts must retain
those separate, potentially unlinked identities.

The parsed policy has a SHA-256 revision over normalized, sorted entries. Origin
root-slash differences and entry order do not change the revision; changed
bindings do. Resolution returns independent copies. Later observation caches
must include this revision plus account and repository scope, and must recheck
current bindings before releasing data. A configured mapping alone is never
proof that a cached observation or a live credential remains authorized.

## Read-route authentication observations

[`CheckReadAccess`](../webmux/server/internal/maccoord/read_access.go) resolves
the receiving account and server-verified repository before making any request.
It accepts a credential supplied by trusted server plumbing and an optional
operator-owned CA pool; it never discovers native profiles or reads token files.
Three GETs to the same fixed project-graph route check absent credentials, a
fresh invalid credential, and then the intended credential. Both negative
requests must return 401/403. The positive response must have the exact supported
schema and selected project key. Graph `found` and unrelated graph content are
not copied into the result and never prove project existence.

The method uses a five-second total context budget, no retries, no ambient proxy,
no redirects, normal TLS verification and bounded response headers/body. It
returns no evidence on cancellation, refusal, unsupported schema, malformed or
ambiguous JSON, wrong project, excessive body size or untrusted TLS. Errors omit
credentials, endpoints and response bodies while preserving cancellation through
`errors.Is`. Every call repeats the authentication observations, so an earlier
success cannot bypass a subsequent credential refusal.

The result is `webmux.mac.read-authentication.v1`. It records the binding revision,
authority/project selection, local observation time and the three read-route
outcomes. Principal identity stays `unverified` and other operations stay
`unqualified`. It is not a worker identity, enrollment receipt, least-privilege
scope audit, lease, admission or publication grant. The selected origin still
needs trusted deployment provenance: allowing a configured loopback URL does not
verify an SSH tunnel's host key, publisher or continued ownership of that port.
These checks are prerequisites before supplying a real credential; status-code
controls alone cannot authenticate an arbitrary local listener.

Controlled Go HTTP/TLS tests cover normal refusal/success, anonymous and
arbitrary-bearer configurations, changed credential acceptance, foreign accounts,
redirects, exact wire fields, size bounds, TLS trust, cancellation and data
redaction. Deliberately removing either negative control or exact policy-field
checking causes the relevant controls to fail. This is client-contract evidence,
not live MAC qualification. The local profile metadata audit found no current
dedicated read-only profile on a qualified transport. No token files were read,
no connection to that fleet was attempted, and no credentials were created or
revoked. Actual authority/transport/scope and revocation qualification remain open.

## Ordered implementation

The following are sub-issues of #664 with actual GitHub blocker relationships;
creating this plan does not dispatch parallel fleet work:

1. [#673](https://github.com/jordanhubbard/webmux/issues/673): versioned identity
   and operation-capability boundary. This primitive is a first implementation
   component; remaining wire mappings and integration qualification stay open.
2. [#674](https://github.com/jordanhubbard/webmux/issues/674): trusted read
   projection shared by human and session-agent consumers.
3. [#675](https://github.com/jordanhubbard/webmux/issues/675): ordinary/managed
   interactive participation through MAC's execution authority.
4. [#676](https://github.com/jordanhubbard/webmux/issues/676): authority-side
   scope/resource and source-roadmap admission.
5. [#677](https://github.com/jordanhubbard/webmux/issues/677): project-native
   review/publication and stale-attempt refusal.
6. [#678](https://github.com/jordanhubbard/webmux/issues/678): real three-host
   human/agent acceptance and lifecycle/failure controls.

The current binding primitive does not complete #673, #674 or the overall
convergence outcome. In particular, it does not yet answer the four product
questions or reconcile existing participants and execution backends.
