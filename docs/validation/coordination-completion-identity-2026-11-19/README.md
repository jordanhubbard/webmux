# Native completion evidence in the coordination API

Refs [#623](https://github.com/jordanhubbard/webmux/issues/623) and
[#246](https://github.com/jordanhubbard/webmux/issues/246). WebMux previously
ignored Agent Deck's `turn_fingerprint` and event `profile` in both inbox Peek
and remote Drain. Both paths now retain the original evidence through the
`webmux-coordination/v1` response and its frontend type.

| Optional message field | Meaning |
| --- | --- |
| `turnFingerprint` | The native opaque `turn_fingerprint`, copied without deriving or normalizing an identity. |
| `sourceProfile` | The native record's `profile`; missing values remain absent. |
| `inboxProfile` | The recipient profile selected for the native Peek/Drain command; separate from the source profile and absent when unknown. |

The existing provider, remote alias, child and recipient fields remain attached.
A fingerprint is not globally qualified, a remote alias does not prove an
unchanged host, and these fields do not authorize delivery or establish an
atomic completion effect. WebMux does not replace a missing native fingerprint
with a timestamp, message body or invented ID. Peek remains observational;
repeated records are returned without consumption or new broker deduplication.
No bridge capability is advertised or enabled.

## Native local check

The verified published Agent Deck 1.16.39 macOS ARM64 binary creates one isolated
shell parent and two owned one-shot tasks through its supported `run-task`
wrapper. The actual task exits are 0 and 7, producing native finished records
with `ok` and `fail` outcomes and distinct nonempty fingerprints. This is ordinary
shell-task completion, not a simulated coding CLI or an authenticated model turn.
Native owner Stop succeeds and the parent payload exits.

The current Go 1.27.4 macOS ARM64 development WebMux binary then reads those same
retained records through two actual loopback HTTP inventory requests, with an
unrelated ambient Agent Deck profile. Both reads preserve the exact native
fingerprints, source profiles and selected `work` inbox profile. Native Peek
before and after returns the original records unchanged. The tasks and parent
are not restarted for these reads. The API process exits normally and the native
parent remains stopped. Both private trusted-mode and default local-authentication cases pass. In the
local-auth case, the original default-auth fixture home still rejects an
unauthenticated inventory request (401). A temporary local fixture administrator
can read both exact native records; a fixture non-admin is refused (403).
Both authenticated reads leave the native inbox unchanged. Temporary HTTP
credentials stay out of reports; no coding-provider login occurs. These local
checks do not establish native remote-host Drain, coding conversation or complete
bridge acceptance.

Three fixture setup attempts remain failed and retained: a development launch
without an asset root, a launch using default authentication because fixture
configs were outside the `config/` directory (401), and an explicitly disabled
integration returning no provider inventory. The final review supplies the asset
root, seeds the intended private config location and enables the chosen Agent
Deck integration. It uses the original two native records. The default-auth 401 remains a correct denial and is separately verified in the
successful authenticated case. Product guards and deadlines are unchanged; each
failed configuration retains its original outcome.

The development binary reports `vcs.modified=true` on base `ef63792`. The
[receipt](receipt.json) binds its actual hash, platform, pinned producer-source
blobs and qualified product-file hashes. This is source-bound development
qualification, not a clean signed package or release qualification.

## Regression and negative controls

The committed HTTP regression covers both Peek and Drain with `work` and
`default` recipient profiles despite an unrelated ambient selection. It checks
two records carrying the same native fingerprint, a distinct turn with identical
display text/time, opaque non-hex characters, and legacy records with no native
identity/profile. All records remain visible; missing native values stay absent
instead of borrowing the recipient profile. Native routing and ordinary message
fields remain intact.

The original parent implementation fails that regression. Disposable Go source
overlays dropping only the fingerprint, only the native profile or only the
inbox profile each fail as well. The fixture log verifies profile-scoped native
Peek/Drain calls and no send or consuming `inbox drain` command. These controlled
remote envelopes verify HTTP mapping, not native remote-host acceptance.

Affected HTTP/config/session race suites, full server vet, all five strict
TypeScript projects, frontend lint, all 472 frontend tests, the frontend build
and native server build pass. Existing test sources and workloads are preserved.
Raw commands, native records, failed setups, overlays, sources and executable
bindings remain under `_build/coordination-event246-current/`.

The full #246 bridge remains incomplete: it still needs a supported Rig-side
adapter, explicit opt-in topology and verified host/provider/profile/native
binding, one lifecycle owner, authorization/provenance and durable correlated
receipt handling across duplicate and uncertain dispatch. The
[owner-queue send intent](../coordination-send-intent-2026-11-19/README.md) remains
a separate prerequisite. Broader recovery and release acceptance remain open.
