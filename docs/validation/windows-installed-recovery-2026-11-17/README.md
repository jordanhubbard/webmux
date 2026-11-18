# Windows installed-service recovery

Native qualification used the CI-built ZIP/WinSW installation from main
`8f5d3225fac65804554a65bc08381a03a70654fe` on the authorized Windows 11 Pro AMD64
VM (10.0.26200.9974). The service ran as LocalSystem with an isolated application
home and project. Existing target-local Codex authentication was used in place;
authentication files and tokens were not exported.

The [packaging run](https://github.com/jordanhubbard/webmux/actions/runs/44125272035)
binds the executable to that source. This was an existing normal CI artifact,
not a rebuild to change its detection result. Archive and executable hashes are
recorded in [the receipt](qualification.json).

## Observed behavior

- Checksum verification and a normal Defender custom scan passed. Real-time
  protection remained enabled.
- The official service installer, health endpoint, authenticated browser login
  and actual ConPTY shell input/output worked.
- Authenticated Codex 0.153.0 continued a synthetic native conversation in its
  original project. A new question omitted the remembered marker and received
  that marker in a fresh native assistant response.
- Forced application-controller termination triggered service recovery. The
  replacement became healthy in approximately 13 seconds. Native owner, root and
  actual Codex process identities were unchanged; another native response
  included the remembered marker and a fresh nonce.
- An actual Windows guest reboot removed those processes. Windows subsequently
  started the configured delayed automatic service. The original native-file
  prefix and both pre-reboot transcript prefixes were byte-identical. No coding
  process started automatically.
- The browser showed waiting recovery with the original conversation/project.
  Clicking **Resume codex conversation** consumed the recorded request and
  started the original CLI directly, without an initial prompt, arbitrary
  command replay or transcript handoff.
- A fresh question received the remembered marker and a new nonce in that same
  native conversation. Repeating the observed browser request returned consumed.
- The official service restart preserved the recovered native owner, root and
  actual coding process. Repeating that recovery request after restart again
  returned consumed; a fifth native assistant answer included another fresh nonce.

Native turn records preserve the project, model `gpt-6-astra`, `read-only`
sandbox and `never` approval policy. Invocation and recovery preview retain the
explicit Windows `unelevated` sandbox setting. Recovery uses the recorded
profile, including its original `CODEX_HOME` and `USERPROFILE`.

## Evidence and limits

Private complete receipts, native audits, browser responses/screenshots, raw
guest-agent results and hashes remain in `_build/windows-service-current/`.
The public receipt contains synthetic identities and summarized evidence only.
Initial collectors were corrected for the native response-item schema, the
normal CLI observation alias and Windows sharing of live log files. Claims
above require final native audits; launcher exit alone is insufficient.
Cleanup removed the temporary service, its service directory and firewall rule,
and verified that the owned owner/root/CLI processes exited. Private synthetic
conversation and diagnostic evidence were retained. Protection stayed enabled;
no matching component detection was recorded, and the executable hash was unchanged.

This qualifies this ZIP/WinSW LocalSystem installation and Codex version/settings.
It does not qualify every service account, CLI, policy or profile, Windows ARM64
hardware, an upgrade to a different executable, power loss, or complete
interactive coding-tool execution. Codex reported a missing optional code-mode
host and retained its fail-closed behavior; the synthetic conversation turns
requested no tools and made no tool calls.

The earlier image classified under [#384](https://github.com/jordanhubbard/webmux/issues/384)
was neither restored nor re-executed. These observations do not establish the
classification's cause or resolution for that image. No exclusions, protection
changes, signing enrollment or security-policy bypass was used. Broader
[#318](https://github.com/jordanhubbard/webmux/issues/318) remains open, and the
dependency-audit merge/release gate [#429](https://github.com/jordanhubbard/webmux/issues/429)
remains required.
