# FIFO heavy-worker admission

WebMux owns the durable queue and the actual fenced lease in one transaction.
MAC and Natasha are clients of this authority. They must not maintain another
lock or an independent ordering queue. The existing `node:<node-id>:heavy`
resource has capacity **one**, shared across projects and authorized accounts.
CPU and GPU heavy work currently use that same capacity; this change does not
add separately configurable CPU/GPU pools or multiple slots.

The authoritative resource in `coordinate view` advertises
`queue_contract: "webmux-worker-queue/v1"` and `capacity: 1`. A client must check
that advertisement rather than assume a newer CLI upgrades an older controller.

## Executor identity

Use an absolute path to the qualified CLI. A stale `webmux` earlier on `PATH`
may predate the `coordinate` subcommand. The active service-tree installation
on sparky is `/home/jkh/Src/webmux/webmux/bin/webmux`.

The actual execution actor must run these commands in its own provisioned
WebMux environment. Set `WEBMUX_COORDINATION_EXPECTED_SESSION` to its intended
session UUID. The supported client loads the private capability internally and
refuses a subject mismatch before any HTTP mutation:

```sh
WEBMUX_COORDINATION_EXPECTED_SESSION=SESSION_UUID /qualified/webmux coordinate guide
```

The guide returns `{contract, subject: {node_id, owner, session_id, incarnation},
instructions}` without a token. Verify the subject. A persona outside that
executor can submit intent through MAC but cannot acquire on the executor's
behalf, copy its capability, or substitute administrator credentials.

## Operations

Send one JSON object on stdin to `/qualified/webmux coordinate act`. It uses
the existing authenticated `/act` API and lease protocol header. All operations
require a stable `request_id` (8–128 characters) and `resource`.

| Action | Additional fields | Meaning |
| --- | --- | --- |
| `enqueue-reservation` | `lease_seconds`, `wait_seconds` | Create a durable ticket with server-assigned order. |
| `inspect-reservation` | `record_id` | Observe ticket status, position and readiness. Never acquires. |
| `acquire-reservation` | `record_id` | Acquire only if this ticket is the available head. |
| `cancel-reservation` | `record_id` | Cancel waiting work. Does not revoke a granted lease. |
| `renew-reservation` | `record_id`, `fence`, `lease_seconds` | Renew the actual existing lease. |
| `release-reservation` | `record_id`, `fence` | Release that lease and complete its ticket. |

`lease_seconds` defaults to 120, range 15–900. `wait_seconds` defaults to 900,
range 15–86400, and establishes an absolute authority-owned waiting deadline.
Polling never extends it. Tickets also expire when their local actor disappears,
is revoked, or changes project. Remote authorization is rechecked on each
exchange; an unreachable remote waiter's bounded deadline prevents permanent
head blocking. Current holders retain their authorized renewal and release
rights; waiting does not preempt running work.

A queued response looks like:

```json
{"status":"queued","record_id":"TICKET","state":"waiting","queue":{"sequence":42,"position":2,"deadline":"2026-11-28T02:00:00Z","state":"waiting","ready":false}}
```

Only an acquisition response with `status: "reserved"`, an original `record_id`,
positive `fence`, `lease.current: true`, and an unexpired conservative `expires`
authorizes work. A queue position, `ready: true`, or a historical grant does not.
Keep the original acquisition request ID for exact retries after a lost reply.
Receipts are frozen: use fresh request IDs for new observations and attempts.
An actor has at most one waiting ticket per resource; additional enqueue IDs
return its existing place. Legacy `reserve` returns `busy` whenever any valid
waiter exists, including a waiter owned by the legacy caller.

The generic `act` CLI continues to return a nonzero exit status for queue-only
outcomes; the JSON response still contains the admitted ticket or observation.
Adapters must inspect that response and must never interpret queue admission
as an execution grant.

## Bounded waiting

MAC can run this single bounded subprocess instead of asking a model to poll:

```sh
/qualified/webmux coordinate wait --resource RESOURCE --record-id TICKET \
  --request-id UNIQUE_WAIT_ATTEMPT --timeout-seconds 20
```

The timeout range is 1–30 seconds. The client observes once per second through
the same authenticated API and returns the last queue response. Success means
the available head can attempt acquisition; acquisition remains a separate
operation. Timeout leaves the ticket in place until cancellation or its waiting
deadline. MAC may schedule another bounded wait with a fresh attempt ID.

## Persistence and deployment

The first enqueue upgrades the retained authority ledger to schema 4 without
changing existing holders, fences, capabilities, history epoch or receipts.
Older controllers reject that schema rather than ignore queued waiters. Roll
forward with schema-4-aware code; do not discard the ledger to roll back.

Qualify a separate candidate before changing a live controller. Preserve
holders and running terminal/provider processes during any coordinated
controller handoff. Afterward, require a real three-actor FIFO canary on the
advertised authority before releasing dependent KSE qualification. Candidate
tests and queue receipts alone do not establish deployed GPU execution, persona
delivery, or Jira acceptance. Natasha's disabled persona handler must be handled
separately without replaying historical pending directives.
