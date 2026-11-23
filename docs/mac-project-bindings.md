# MAC observation binding contract v1

Implementation foundation for [#673](https://github.com/jordanhubbard/webmux/issues/673)
and the [project authority contract](mac-project-coordination.md). The Go package
[`internal/maccoord`](../webmux/server/internal/maccoord/bindings.go) parses and
resolves this versioned operator document. It has no file loader, environment
discovery, network client, credential access or dispatch path. No product setting
currently enables this mapping. The read adapter and its human/agent consumers
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
unknown fields, duplicate JSON keys (including escaped equivalents), invalid
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
