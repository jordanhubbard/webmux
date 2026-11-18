# Agent Deck profile routing — 2026-11-17

Candidate repair for [#448](https://github.com/jordanhubbard/webmux/issues/448).
This is adapter routing qualification, with deterministic CLI fixtures. It does
not qualify native coding-provider delivery, completion, remote authentication,
restart/reboot, or the proposed Rig/Agent Deck bridge in #246.

## Problem and behavior

The native inventory can return sessions from multiple local profiles.
Attachment and ownership already use a discovered nondefault profile, but inbox
peek, guarded message send, delivery lookup and remote completion drain omitted
it. Those operations could resolve through the CLI's default or inherited
profile. An explicit `default` profile was also omitted by the existing argument
helper and attachment path, so `AGENTDECK_PROFILE` could override that selection.

All four message/completion paths now pass the uniquely resolved local profile.
Attachment and the shared ownership/recovery argument helper also preserve an
explicit `default` choice. Empty profile metadata keeps its existing CLI-default
behavior. Remote-owned session send/lookup/attach retain their registered-remote
routing; a remote's profile is not applied as a local profile. Completion drain
uses the local receiving conductor's profile and preserves source-remote
attribution. Native guarded queue flags and stdin-only message bodies remain.
This does not change a remote's configured profile compatibility policy.

The profile precedence was inspected in the published Agent Deck 1.16.39 source,
commit `d62c2898d623e5353392a06f9a77c19f97b33438`:

- [Global profile selection](https://github.com/asheshgoplani/agent-deck/blob/d62c2898d623e5353392a06f9a77c19f97b33438/cmd/agent-deck/main.go#L332)
  applies explicit `-p` to the command's `AGENTDECK_PROFILE`.
- [Non-consuming inbox reads](https://github.com/asheshgoplani/agent-deck/blob/d62c2898d623e5353392a06f9a77c19f97b33438/cmd/agent-deck/inbox_peek_cmd.go#L15)
  resolve through the explicit profile contract.
- [Remote drain](https://github.com/asheshgoplani/agent-deck/blob/d62c2898d623e5353392a06f9a77c19f97b33438/cmd/agent-deck/remote_drain_cmd.go#L248)
  uses its effective profile for the configured same-profile policy; the
  existing native policy and cross-profile refusal remain authoritative.

## Deterministic acceptance

The adapter fixture accepts the exact expected command arguments and otherwise
fails. Two cases advertise `work` or `default` while the controller inherits an
unrelated profile. Each verifies attachment arguments, ownership, inbox read,
guarded queued send, sanitized delivery lookup, remote drain and provenance.
The captured message body arrives only on stdin. Duplicate IDs across local
profiles refuse send, lookup and drain before a messaging mutation is invoked.
Existing default/empty-profile and remote routing cases remain in the full suite.

The exact final test source is also run against unchanged main `8f5d322` in a
separate qualification checkout. It fails for the work-profile inbox read and
explicit-default attachment. Only the test file is temporarily overlaid; runtime
sources are untouched, and the original test file and clean checkout are
verified after restoration. This is an adapter negative control, not native
provider evidence. See [parent-negative.json](parent-negative.json).

The [source manifest](source-manifest.json) binds source, accepted checks and the
negative receipt. Review, exact-source hosted CI, audit #429 and the broader
release gates remain required. The bridge remains unimplemented and opt-in work
under #246.
