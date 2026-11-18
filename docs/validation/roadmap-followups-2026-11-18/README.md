# Combined roadmap follow-up candidate

[#452](https://github.com/jordanhubbard/webmux/issues/452) composes the candidate
without merging or changing the released `main`. All pinned inputs remain Git
ancestors; merges resolved without conflict.

| Input | Retained source |
| --- | --- |
| Original combined candidate #453 | `10b365c6a5405848d1630d6fbd2f096a830a5998` |
| Mobile companion #455 | `52a8527e7e8a3e0ed0d0af09eadbd9457eae9854` |
| Native USB qualification #460 | `3b80d0d4174a366f8fc0a3d0175b30e0323377b8` |
| Native ARM qualification #457 | `447554456ff5e1dde3000b13c5d08e35bbfdee69` |
| Broadcast recipients #463 | `8e90c3ba68f1e037c592a24b4c22e463cc53e071` |
| Mosh browser control #472 | `b357ca694b390bbca1377a0a759d44ede0331220` |
| Native inventory and regular-file admission #467 | `49b1809c94296db7ee1f3c5dd962c00cc6f1bc31` |
| Cleanup diagnostics #475 | `4ce5d90` |
| ARM renderer scheduling #476 | `fd1a44161e9735509e8c02cbdc36e9143e1c46d1` |

Production Go and frontend source remain unchanged from the Mosh candidate.
The CI merge preserves every prior step/condition and adds the inventory controls,
live guacd inventory and artifact retention. Linux installs Mosh before that
inventory, allowing the combined run to observe the actual installed tools.
The ARM command uses two isolated renderer workers; its 1,384 cases, per-test
assertion limits and 60-minute aggregate job budget remain unchanged.

The native Mosh fixture initially failed after seven other cases passed. Its
worktree-derived socket path was 110 bytes; native tmux reported `File name too
long`. The previously qualified shorter worktree produced a 101-byte path. Host
state had browser control enabled but no durable provider, and only the initial
recovery RPC was recorded. This is a fixture-path admission failure, not a new
application-source repair. Its failed log, receipt and path diagnosis are retained.

The fixture now creates its private socket directory below the local temporary
root. It requires the actual durable tmux provider before beginning sign-in and
verifies the explicit owned socket is stopped before reporting cleanup. Arbitrary
tmux errors no longer establish absence. Directory removal occurs only after the
provider, Mosh server and SSH daemon have exited. Other private fixture data stays
in the worktree fixture home. All original sign-in, no-URL, logging-off,
reconnect/acknowledgment and deadline assertions remain.

Local qualification passes: full root server race tests and vet, 410 frontend
checks, 92 helper checks, strict source coverage/typechecks, frontend lint/build,
11 inventory controls and real macOS tool inventory. After the fixture change,
strict checks, native private-tmux ownership/cleanup controls, the final build and
all eight real macOS browser cases pass (21.8 seconds). The Mosh receipt's ten
native/callback/privacy/acknowledgment/cleanup flags are all true. The later
browser-test-only identity delta has its own race/vet/privacy and Windows
AMD64/ARM64 cross-compilation evidence in #475; compilation is not native execution.
[Bindings and scope](source-manifest.json) distinguish those snapshots. An initial
typecheck failed because the new worktree lacked its frontend dependency link;
that environment failure is retained and required no source change.

This remains a draft qualification candidate. Cleanup #473 has reproduced native
sharing refusals; diagnostics do not repair them. Exact native identity correlation
and the two-worker ARM gate remain pending. Audit #429, broader native/authenticated
acceptance, integration review and exact-main release gates remain required. The
newer Rig Stop branch is intentionally not composed while its own follow-up gates
are under investigation. No version, tag, release or `main` mutation is made.
