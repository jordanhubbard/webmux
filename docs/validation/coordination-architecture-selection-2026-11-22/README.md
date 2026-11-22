# Coordination architecture selection evidence

The [architecture decision](../../coordination-architecture.md) completes the
architecture-evaluation roadmap item under [#656](https://github.com/jordanhubbard/webmux/issues/656).
The remaining implementation and full workflow items under #630/#631 stay open.

The repeatable [POSIX qualifier](../../../scripts/qualify-coordination-provider-interfaces.py)
starts actual installed provider MCP entrypoints and a current-source WebMux CLI
under empty private HOME/XDG/native-config roots. It inherits no native identity,
authentication or API-key environment. Rig uses its installed Node/entrypoint
directly because the installed shell wrapper forces a live profile. An explicit
port points only at the qualifier's refusing loopback endpoint; no daemon starts.

| Interface | Native result | Controlled read |
| --- | --- | --- |
| Rig 0.6.14 (`9d0200f3`) | Initialize, MCP 2025-11-25 and 23 advertised tools | `rig_status` reaches the owned `/healthz`, receives 503 and reports a tool error. |
| Agent Deck 1.16.34 Recall | Initialize, MCP 2025-06-18 and three advertised tools | `recall_show` reports the absent record in an empty private index as a tool error. |
| WebMux source `3cacedc` | Initialize, MCP 2025-11-25 and two scoped coordination tools | `webmux_coordination_view` reports unavailable private access as a tool error. |

Initialization/tool discovery makes no backend requests. The only HTTP request
in the entire probe is Rig's controlled read to the owned endpoint. Only those
three fixed read-only tool calls are allowed by the driver. No lifecycle,
terminal-send or queue mutation is invoked. Agent Deck's separate catalog query
reads only its seeded non-executable fixture entry. A fresh disabled-Recall
profile refuses with exit 2; enabling Recall occurs only in the empty positive
fixture profile. No existing profile or conversation is read or changed.

The first private prototype omitted that Recall opt-in and its protocol stream
ended before initialization. Its stderr was not retained because the prototype
saved diagnostics only on success. The published qualifier records diagnostics
on every exit; the separately retained disabled-profile control establishes the
normal off-policy refusal. The final complete probe and all controlled refusals
pass. Reusing an output directory fails without changing the original receipt;
Python optimization is refused before creating output so assertions remain active.

The [receipt](receipt.json) binds the executable, qualifier, inspected source and
raw protocol hashes. Agent Deck's binary VCS revision matches the inspected
release source; Rig's actual installed entrypoint/tool implementation is hashed.
The WebMux binary is built from the stated source revision. Existing native
provider lifecycle and ordinary-agent workflow receipts retain their separate
versions/source boundaries. This comparison establishes available interfaces and
the architecture choice; it does not qualify every provider tool or complete
managed-agent participation.

To repeat on a POSIX host with the selected binaries already available:

```bash
python3 scripts/qualify-coordination-provider-interfaces.py \
  --output <new-private-output-directory> \
  --rig-node <absolute-installed-node-path> \
  --rig-entry <absolute-rig-dist/bin-wrapper.js> \
  --agent-deck <absolute-agent-deck-binary> \
  --webmux <absolute-webmux-binary>
```

The output's parent directory must exist. Keep the produced profiles, protocol
records and receipt private. The qualifier preserves earlier outputs and checks
that the selected executable/script files remain unchanged during the run.
Windows requires its own native profile/custody qualifier and is rejected here.
