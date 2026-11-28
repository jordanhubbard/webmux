# Native credential hub contract

An administrator account may choose one existing, enrolled HTTPS WebMux peer as
its AI credential hub. MAC is not required. Peer discovery, terminal access and
coordination permissions do not authorize credential retrieval. This feature
initially supports portable OpenAI and NVIDIA bearer API keys used only by the
server-side AI assistant. Native CLI login profiles, device-bound OAuth, private
keys, arbitrary environment injection and credential replication are unsupported.

## Identities and permissions

A secret belongs to one hub account generation. The private namespace includes
the authenticated username and salted password-hash generation, so removing and
recreating an account or resetting its password cannot revive old secrets or
handles. Re-enabling the hub for a new account generation requires entering its
keys again; old encrypted generations remain inaccessible and count toward the
bounded storage limit. A separate secret grant binds its immutable
ID to one explicit incoming federation grant, the grant's independent
enrollment generation, the administratively bound sending node/account and one
consumer (`ai.openai` or `ai.nvidia`). The AI consumer is account scoped, with no
project selector; a nonempty project selector is refused. The incoming grant must
separately enable credential sharing. Removing/rebinding/replacing that enrollment,
revoking the secret grant, removing an account, disabling or rotating the secret
invalidates outstanding handles. Recreating an enrollment does not revive grants.

The existing TLS exporter proof pins the **receiving hub** node, account and TLS
key on the same connection used to transmit the incoming bearer. Destination
identity headers do not authenticate the sending machine. A dedicated incoming
bearer, administratively bound to the sending node/account, authenticates its
holder. Tokens must be unique and not reused between recipients. Recipient
identity is derived from this server-side grant, never asserted by request fields.

## Custody and delivery

The hub must be explicitly enabled through HTTPS or an authenticated loopback
connection. Only administrator accounts may configure or consume hub credentials
in this initial implementation. One server process per WebMux home is supported. On Linux and macOS it stores an independent
random 256-bit AES-GCM key in an owner-only regular file inside a private 0700
vault directory, separate from the encrypted document. File ownership, modes,
symlinks, sizes, schema and ciphertext authentication are checked. This protects
against other OS accounts and accidental disclosure of the encrypted database;
it does not protect against the owning OS account or host compromise. Anyone
who can execute commands as the WebMux service user, including terminal users,
can read that key material. Per-account API grants are not an OS isolation
boundary; use this hub only within that trust boundary. It is not
hardware-backed/Keychain custody. Unsupported platforms expose unavailable hub
storage rather than plaintext. Back up both files privately; loss of the key
makes the vault unavailable, and no replacement key is silently generated over
existing encrypted data. Old configuration and provider profiles are untouched.

Issuance returns only a random, bounded, short-lived handle. Redemption consumes
it atomically before returning bytes over the pinned TLS connection, rechecking
all current permissions and the credential version captured at issuance. Named
grants continue across rotation; outstanding handles for older versions are refused. Handles are memory-only,
expire after 30 seconds and do not survive restart. The encrypted document retains
bounded redacted audit records, grants and secret versions. Neither normal read
APIs nor the browser receives the secret value. The receiver retains no durable
secret cache: it hands the value directly to the selected AI provider consumer,
using fixed provider URLs, direct transport and no redirects. Hub outages and
uncertain delivery fail closed without ambient-provider fallback. A lost reply
is uncertain; retry starts a fresh request, not an exactly-once promise. Already
delivered provider keys require provider-side rotation/revocation to invalidate.

Account-local hub selections bind the current enrolled peer generation and pin.
Changing/removing/recreating a peer invalidates an old selection. A request already admitted at its final local
authorization check may finish; revocation prevents subsequent admissions. Disabling the
selection restores the existing local AI configuration; a configured unavailable
hub never silently restores ambient credentials. Existing terminal/managed-agent
processes, conversations and lifecycle ownership are unchanged.

## Qualification and rollout

Backend/race, actual pinned HTTPS, UI-to-consumer and three-host canary controls
must establish the positive path and denial for ordinary peer permission,
wrong account/node/grant/consumer/project, changed pin/enrollment, replay,
concurrent redemption, expiry, rotation, disabled secret, removal, restart,
missing/unsafe key custody and unavailable hub. Use private homes and dedicated
fake provider credentials only. Full original make test, make lint, build,
review/CI and exact-main checks remain required. Component tests do not establish
native platform or complete bridge #673–678 acceptance. No real credentials are
enrolled or migrated by deploying this opt-in feature.

## Operator setup

1. Sign in freshly to each server as a local administrator. Older login tokens
   require one fresh sign-in after upgrading to generation-bearing sessions;
   refreshing a token never upgrades its account generation. Running terminals
   and native provider processes continue independently of browser sign-in.
2. Open **Manage servers → Credential sharing** on the receiver and copy its
   displayed node ID and account. On the hub add a dedicated incoming grant,
   enable **Allow credential hub requests**, and bind those two fields. Use a
   unique link credential for this recipient.
3. On the receiver add the hub's HTTPS URL and that link credential, then verify
   its node identity. Each credential request rechecks the enrolled identity on
   the same TLS connection used to transmit that request. HTTP entries cannot
   be credential hubs.
4. On the hub open **Manage shared credentials**, enable encrypted storage,
   enter a named supported API key, and grant it to that receiving enrollment.
5. On the receiver open **Manage shared credentials**, select the hub and
   consumer, load granted names and choose a credential. **Ask AI with shared
   credential** uses the selected key privately on the server. Repeat the
   incoming and named grants separately for each additional receiving account.

Use **Revoke credential grant**, disable/delete/rotate the stored credential, or
remove the incoming enrollment to stop new admissions. **Use local AI
configuration** explicitly removes the receiver's selection. Resetting/recreating
a receiving account invalidates its existing selection; reset/recreation of the
hub account also invalidates its old vault namespace. Existing managed native
agent sessions are not credential consumers in this version: their provider
profiles, conversations and lifecycle ownership remain under the native provider.


The public exchange has a fixed-size in-memory admission window: at most 128
requests per second overall and 32 per source IP, independent of source port.
Unauthenticated or malformed refusals never append durable event records.
Authorized vault operations retain their bounded, redacted audit.

Each current incoming enrollment can hold four live retrieval handles across
all its secrets and consumers. The global ceiling reserves that capacity for
all 256 possible current grants (at most 1024 live handles); expired, revoked,
regranted and rotated handles are reclaimed before admission. Filling one
recipient's allowance does not consume another enrollment's reserved capacity.

Selection changes use a persisted monotonic revision. Removing an absent
selection also advances it. A delayed selection cannot undo a newer removal
or overwrite a newer selection; it receives 409 and requires a fresh attempt.
The commit checks the current authenticated account generation and enrolled
peer fingerprint under the same configuration lock as the selection write.
