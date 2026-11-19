# Native Windows active-tool upgrade and recovery

Actual authenticated Codex 0.153.0 work continues across a private user-run
Windows AMD64 executable replacement, then recovers the original conversation
after provider loss. Durable duplicate controls, sibling retention, filesystem
restrictions and retained history pass. **Overall Windows qualification remains
incomplete:** legacy network/default-shell limitations remain under
[#538](https://github.com/jordanhubbard/webmux/issues/538), and explicit local
stop/discoverability remains under [#541](https://github.com/jordanhubbard/webmux/issues/541).
[#318](https://github.com/jordanhubbard/webmux/issues/318) remains open.

## Actual lifecycle evidence

The fresh case used Windows 11 Enterprise AMD64 10.0.26100, the existing native
Codex 0.153.0 account in place, model `gpt-6-astra`, `read-only`, `never`, and
`windows.sandbox=unelevated`. The qualifier used explicit `cmd.exe` shell tools;
this does not qualify the failing default PowerShell path. Native project trust
was reviewed through the native UI. No native authentication file was copied or
read by the qualifier, no new login was started, and no installed CLI update, sandbox-default,
service, account/firewall, Defender or reboot changes occurred.

The baseline is the actual main `8f5d322` executable from
[packaging run 44125272035](https://github.com/jordanhubbard/webmux/actions/runs/44125272035),
artifact `14346284768`. Its ZIP/executable hashes match the prior retained
[installed qualification](../windows-installed-recovery-2026-11-17/qualification.json).
The replacement is a clean canonical embedded Windows AMD64 build of `5fadbbc`,
with a different executable path and SHA-256. Both API producer reports identify
the expected clean revisions; these self-reports are explicitly unattested.

| Check | Actual result |
| --- | --- |
| Fresh coding before replacement | Native tool reads a fresh fixture nonce through the browser terminal |
| Controller replacement during active native tool execution | Terminate only the verified baseline controller; start the different candidate executable with the same isolated home/cache/listener |
| Survivor first | Nine process identities, including provider, native CLI, code-mode host and active Python tool, retain PID, creation time and image hash; browser reattaches without Resume |
| Active tool completion | One launch, seven waits on original tool session `95007`, matching START/END nonce, exit 0 after the qualifier releases the read-only wait script |
| Fresh native work after replacement | No-tool original-marker recall and another actual tool read with a new nonce |
| Owned provider loss | Terminate only the verified private provider owner; explicit Reconnect opens the recovery wait screen without launching a fresh coding conversation |
| Public native Resume | Original conversation selector, original project/model/policy/profile roots, no initial-prompt replay; action returns `consumed` |
| Fresh work after Resume | No-tool marker recall; native Python tool reads a new nonce, reports original roots and receives `EACCES` for both canary writes |
| Durable duplicate requests | Repeat the identical native action before and after another controller crash/restart: both return `consumed`; provider and native CLI identities stay unchanged |
| Sibling/viewers | Sibling provider and shell persist, its environment marker is returned, and two browser pages remain open through provider loss and the second controller restart |
| History | All 28 recorded prefix checks match; six terminal transcript files and the original native journal are retained |

The native journal has twelve completed turns in total, including four earlier
[preflight turns](../windows-coding-preflight-2026-11-18/README.md). The two
post-interruption recall turns contain no tools. The active tool's original
process exists on both sides of replacement before release; the completion
comes from its original native tool session. These checks go beyond remembered
assistant text or fixture-only Windows CI.

Controller restarts add transcript files while preserving the old files and
prefixes. This is not a claim that a restart appends to one unchanged active log
path. The [receipt](receipt.json) records executable/native hashes, process and
session identities, outcomes, prefix counts, probe-script hashes and the frozen
private evidence-manifest hash. The 219 private raw/source files retain actual
journals/transcripts, action/browser receipts, PID/creation/image snapshots,
drivers and failed attempts. No native authentication files are included.

## Cleanup and remaining limits

Public DELETE returned 204 for both sessions and removed their saved entries,
but retained their durable owners/jobs. Source inspection confirms that the
native viewer's Close detaches; production has no call to `nativeprovider.Stop`.
The initial cleanup check therefore **failed**, with five private resource
holders. That raw result is retained. This behavior requires the lifecycle and
discoverability work in #541; it is not a product teardown pass.

The qualifier then stopped only the two private owners after verifying their
PIDs, creation times, paths and image hashes. Final checks find all 23 recorded
processes gone, both private listeners closed, no denied canary files, successful
matching direct-write controls, and no Windows Restart Manager holders for ten
fixture image/log files. All evidence remains retained.

Other retained limitations:

- The earlier default PowerShell tool failed `0xc0000142`. The preflight private
  loopback-denial assertion failed; external-network denial was not measured.
  This case does not turn either failure into a pass.
- An initial absolute-executable launch bypassed WebMux capture. It was exited
  cleanly before the normal captured command resumed the same native ID.
- Some native TUI prompts remained pending after the first CR. Only observed
  pending input received another Enter; raw input frames and attempts are kept.
  The cause remains unproven. No prompt was blindly resent.
- The generic browser footer overwrote the sibling screen text path. The audit
  instead verifies its actual marker-output bytes in the retained sibling
  transcript, rejecting the earlier set-command/replay as proof.

This private user-run case does not qualify an installed-service/MSI upgrade,
other service accounts or native CLI/policy combinations, authenticated ARM64,
a new host reboot, the unresolved private Windows VT timeout, or full roadmap
and release acceptance. The existing installed-service/reboot receipts remain
separate evidence with their original scope.
