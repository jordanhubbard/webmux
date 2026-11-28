# Native credential hub qualification after adversarial review

The production API and both supported private AI consumers ran on three
distinct native hosts: Linux AMD64, Linux ARM64 and macOS ARM64. Each host
served as designated hub while the other two received explicitly granted keys.
All 103 controls passed in every rotation.

Fixture executables were built from clean source
`bcb5202ec6d31086926b6c2e825196aa5d4861d7`. Every running executable reported
that exact unmodified source and the expected native OS/architecture. The
orchestrator also verified three distinct host identity hashes. These JSON
receipts contain role names and hashes, with addresses, targets, local homes,
credential values and bearer/handle values excluded.

| Hub | Passed checks | Receipt |
| --- | ---: | --- |
| linux-amd64 | 103 | [hub-linux-amd64.json](hub-linux-amd64.json) |
| linux-arm64 | 103 | [hub-linux-arm64.json](hub-linux-arm64.json) |
| darwin-arm64 | 103 | [hub-darwin-arm64.json](hub-darwin-arm64.json) |

The original 99 controls remain, covering explicit grants, independent recipient
and account/consumer/project scope, changed node identity and destination pin,
same-connection TLS proof, one-use concurrent redemption, real expiry,
rotation/disable/removal, independent revoke/regrant, enrollment/binding
replacement, restart, audit/metadata redaction, private encrypted custody,
missing/unsafe key files, no durable receiver cache and no ambient fallback.
Four additional controls fill one recipient's four-handle allowance, require 429
on its next issue, prove the other native recipient's private consumer remains
usable, and redeem the reserved handles. These fresh-capacity controls run
before intentional identity faults that may abandon an issued handle until TTL.

Adversarial review also drove deterministic Go race regressions for distributed
unauthenticated traffic causing no durable event or vault writes, bounded source
admission including IPv4/IPv6 normalization, all 256 stored grant recipients
retaining their handle capacity, invalidated-handle reclamation, and real pinned
TLS selection interleavings with removal/replacement/account/peer changes. Stale
deletes cannot remove a newer binding. A persisted revision and final account
and peer checks fence commits under the same configuration lock.

Original full `make test`, `make lint` and `make build` passed at the clean
qualified source, with source cleanliness and identity checked before and after
each command. Existing platform skips and assertions were unchanged. Full test
time was 640.847 seconds. The published, checksum-verified Node 1.3.11 archive
seeded a private upgrade home; the newly built Go server verified fresh login,
rejection of old JWT access/refresh and unchanged saved hosts/auth/app config.

The OpenAI and NVIDIA consumers use a dedicated synthetic TLS provider and
test-only keys. These records do not establish paid-provider acceptance, native
CLI/OAuth/profile portability, hardware-backed custody or physical desktop
capture. File custody trusts the service OS account as documented in the
[contract](../../credential-hub.md). No production service or real provider
credential/profile was changed by qualification.

The [initial 99-control checkpoint](../credential-hub-2026-11-28/README.md) is
retained as historical evidence. Independent review of the final head, CI,
exact-main validation and deployment remain separate gates; a later docs-only
commit does not extend this evidence to unrelated source changes. MAC remains
optional and broader authority-bridge acceptance remains open.

## Receipt SHA-256

- `hub-linux-amd64.json`: `08801a9bd97b243f71bf300032e69c7d29c18e748bfc354703de263c96047ade`
- `hub-linux-arm64.json`: `9dce858b3ef860d7fccd147c80321bfb4367a827b4e47faf60c0932dadd4b79c`
- `hub-darwin-arm64.json`: `133c6079417ae6a7b5aee182446c02913848d483f6847c380e518f91f17f1bff`
