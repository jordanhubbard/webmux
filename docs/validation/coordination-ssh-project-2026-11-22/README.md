# Authenticated SSH repository context

Under [#630](https://github.com/jordanhubbard/webmux/issues/630), ordinary SSH and
mosh sessions can now verify their observed working directory on the execution
host. Remote project identity previously remained unavailable; inspecting the
same path on the controller would be incorrect. A fixed read-only recovery RPC
now runs the existing Git resolver through the authenticated host channel.

The broker revalidates the live run, session incarnation, account, destination
and directory after network work. Failed or expired observations cannot supply
current project context. Managed attachment viewers remain excluded, and the
remote agent interface still reports onboarding pending. No remote capability
or agent-aware guidance channel is claimed here.

## Native component evidence

The [qualifier](../../../scripts/qualify-coordination-ssh-project.py) ran on native
macOS ARM64 against the production Linux ARM64 `--recovery-rpc` executable over
authenticated SSH stdin. A transferred test executable initializes only a new
private qualification cache with a random session/key and recording disabled.
Neither executable starts a shell, tmux provider, WebMux server or active agent.
No installed helper or service configuration is changed.

The actual production RPC then:

1. Verifies the remote fixture's canonical origin and root commit against an
   independently computed expected project digest.
2. Reports its worktree and `rpc-original` branch, with no recovery events or
   recording content in the response.
3. Refuses a foreign session, foreign authentication key, missing/relative path
   and a directory that is not a repository.
4. Observes `rpc-changed` after an actual remote Git branch change while
   preserving the original repository identity.

The [receipt](receipt.json) retains the successful command's exit code, individual
control outcomes, qualifier/source hashes and executable hashes. Random
authentication travels through encrypted stdin and is absent from arguments and
the receipt. Remote fixture files remain below the private qualification cache;
there are no long-running fixture processes to stop.

## Controls and limits

Focused race tests cover current-run success and refusal after owner, directory,
destination, port, account, SSH-key, provider, cancellation or run changes. Old
helpers and unverified responses fail closed. API controls prove there is no
controller-filesystem fallback even when an identically named local directory is
a valid repository, and verify cache expiry, namespace isolation, global disable
and preservation of other owners' context during a partial refresh. Path
recognition covers Unix, Windows drive-rooted and UNC syntax independently of
the controller OS. Host RPC tests verify authentication with capture disabled
and unchanged host history/revision.

The initial new broker test fixture lost its original cleanup handle in the
replaced-run case and was terminated. The fixture now restores that handle;
the final race controls pass. No production deadline or admission assertion was
weakened. Vet and Windows ARM64 test compilation also pass. Compilation is not
Windows native execution evidence.

The full Go suite also reproduced the unchanged Rig PID-marker readiness race
under #633. Its failed outcome remains in the receipt. The separately qualified
[atomic-publication fix](../rig-change-marker-2026-11-22/README.md), submitted as
[#645](https://github.com/jordanhubbard/webmux/pull/645), preserves that test's
original observer, deadlines and process-exit assertions.
The complete Go suite with Node 24 passes on the final reviewed SSH source with
that identical fixture fix included; the receipt binds its log separately from
the earlier failed run.

This is native RPC/component acceptance. It does not prove an ordinary remote
agent adopted MCP guidance, full broker/provider SSH or mosh operation, managed
agent participation, existing-agent awareness, controller restart/reconnect,
native Windows behavior, or multicast discovery/enrollment. Those requirements
and the complete #630/#631 workflow remain open.
