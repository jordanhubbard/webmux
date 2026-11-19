# Native Windows coding-tool preflight

The planned strict Windows coding-policy prerequisite **failed**. Fresh native
Codex authentication and a real sandboxed file-read tool call succeeded, and
both canary writes were denied. The loopback-denial check failed. No WebMux
controller or executable-upgrade/recovery scenario was started by this preflight.
[#538](https://github.com/jordanhubbard/webmux/issues/538) tracks the limitations;
[#318](https://github.com/jordanhubbard/webmux/issues/318) remains open.

## Native execution evidence

Windows 11 Enterprise AMD64 10.0.26100 ran the existing Codex CLI 0.153.0 in its
existing signed-in account. Four turns in one fresh native conversation retained
the same fresh project, model `gpt-6-astra`, `read-only` sandbox, `never` approval,
and `windows.sandbox=unelevated`. The exact native session journal and native
JSON events were correlated. No native auth file was read/copied by the qualifier,
and no new login or CLI/configuration update occurred.

| Control | Observed result |
| --- | --- |
| Default PowerShell tool | Failed at process start, exit `-1073741502` / `0xc0000142`, empty output |
| Direct PowerShell control | Exit 0, read exact fixture nonce |
| Explicit `cmd.exe`, `login=false`, Python `-c` | Rejected harness attempt: quoting produced Python syntax error; raw retained |
| Explicit `cmd.exe`, `login=false`, fixed Python script | Exit 0, exact nonce returned by actual native tool |
| Native policy script: project and outside-project canary writes | Both denied with `EACCES`; both canaries absent afterward |
| Qualifier's matching write controls | Both succeed, then remove only their own control files |
| Native policy script: private IPv4 loopback connection | **Allowed**, failing the planned denial check |
| Qualifier's direct loopback controls before/after | Both succeed; listener records exactly three accepts including native connection |

The qualifier wrote `probe-read.py` containing only:

```python
from pathlib import Path
print(Path(__file__).with_name('probe-input.txt').read_text())
```

The native tool ran `C:\Python312\python.exe probe-read.py` using
`C:\Windows\System32\cmd.exe` and `login=false`. The policy probe used a separate
fixed script and only disposable canaries within this private qualification root.
Its network target was the qualifier's private loopback listener. This does not
measure external-network denial or establish a host/network escape mechanism.
Native CLI exit 0 alone does not establish tool success: it occurred even for the
failed PowerShell tool.

The SSH observer is in session 0 on a service window station. That is not a
measurement of the sandbox child's desktop. An
[upstream session-0 report](https://github.com/openai/codex/issues/46412) describes
similar failures; this preflight does not prove that root cause here. Current
[upstream permissions documentation](https://learn.chatgpt.com/docs/permissions)
describes weaker network isolation in the legacy unelevated implementation.
The observed loopback result remains a failed assertion for this planned case.

## Upgrade artifact binding and remaining work

The downloaded [packaging run 44125272035](https://github.com/jordanhubbard/webmux/actions/runs/44125272035)
artifact `14346284768` is bound to main
`8f5d3225fac65804554a65bc08381a03a70654fe`. Its combined ZIP SHA-256 and server
executable SHA-256 match the retained
[installed-recovery qualification](../windows-installed-recovery-2026-11-17/qualification.json).
The server-only ZIP contains the same server executable. The outer GitHub artifact
ZIP and nested release ZIP have distinct recorded checksums.

The [receipt](receipt.json) records exact versions, hashes, controls, failures, and
hashes of private native evidence. Raw JSON events, the exact native journal,
drivers, rejected attempts, and fixture state remain retained privately. No
credentials, private account paths, host addresses, or unrelated process arguments
are included in this report.

This does not qualify WebMux recovery, different-executable upgrade, surviving
ConPTY ownership, explicit native resume through the recovery UI, durable duplicate
controls, log-prefix preservation, installed-service accounts, ARM64, or reboot.
Those checks still require actual application evidence. No reboot, service,
Defender, account/firewall permission, or global CLI changes were made.
