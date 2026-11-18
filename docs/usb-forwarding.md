# Experimental USB forwarding transport

Run `webmux usb-forward` on the **computer hosting the USB device** to carry a
native USB exporter's TCP traffic to the receiving computer you use through WebMux.
The remote helper opens an IPv4 loopback listener; a native USB importer on that
computer connects to it. This is a foreground CLI companion in the WebMux binary,
not a browser USB picker or an HTTP device-control API.

The device can be a robot, a USB security key or another peripheral supported by
the selected native backend. A simulated USB device is also supported for testing.

**Status:** transport implementation with synthetic TCP, real OpenSSH and a
Linux kernel USB/IP gadget qualification tier. A simple simulated USB device is
sufficient for roadmap device acceptance. Physical hardware, an iPhone and Xcode
are not prerequisites. Browser lease status and Stop controls use the native
helper's reports; the importer remains responsible for actual device attachment.
Track exact-source qualification and merge acceptance in
[#120](https://github.com/jordanhubbard/webmux/issues/120).

## Why native software is required

A browser does not attach devices to another machine's USB bus. Full device
forwarding needs an exporter beside the device and an importer/driver on the
receiving computer. WebMux supplies the authenticated transport between them;
it neither implements nor bundles those drivers.

Choose compatible native software for the exporter, importer, device and OS
pair. The Linux qualification fixture uses a simulated mass-storage gadget with
the native USB/IP stack. Device-specific application behavior, such as robot
commands or security-key operations, belongs to the device and its driver;
WebMux transports the backend's TCP traffic without interpreting those commands.

## Cross-platform peers and the USB button

The peers do **not** have to run the same OS. The WebMux transport carries TCP;
USB compatibility comes from the native exporter, importer and destination device
driver. Direction matters: the exporter runs beside the device or simulator, and
the importer runs where applications will use it. Reverse those roles to forward in
the opposite direction. A device is normally claimed by one importer at a time.

| Device or simulator computer → receiving computer | Candidate native software |
| --- | --- |
| Linux → Linux | Linux USB/IP exporter and importer, or VirtualHere |
| Windows → Linux | usbipd-win with Linux USB/IP importer, or VirtualHere |
| Linux → Windows | Linux USB/IP with a compatible Windows USB/IP client, or VirtualHere |
| Windows → Windows | Compatible Windows USB/IP exporter/client, or VirtualHere |
| macOS ↔ Linux / Windows / macOS | VirtualHere offers both roles; validate the particular device and OS versions |

These are evaluation options, **not WebMux hardware qualification results**.
[VirtualHere](https://www.virtualhere.com/) supplies servers and clients for all
three platforms. [usbipd-win](https://github.com/dorssel/usbipd-win) is a Windows
**exporter**, not a Windows receiving driver.
[usbip-win2](https://github.com/vadimgrn/usbip-win2) is a separate Windows USB/IP
client. [Linux USB/IP](https://docs.kernel.org/usb/usbip_protocol.html) describes
the native protocol. Both peers must use compatible implementations; do not
assume that a VirtualHere server and a generic USB/IP client are interchangeable.

Click **More → USB redirection**, enter the receiving SSH host, SSH port, local
exporter port and lease duration, then choose **Create local command** and
**Copy local command**. The command pairs the foreground helper with your
dashboard using a one-use code that expires after five minutes. Creating a
command leaves forwarding pending until you run it locally.

The dialog shows your forwarding leases and accepts **Stop forwarding** requests.
The button is available in every workspace and does not intercept terminal keys.
Native exporter/importer software still handles drivers and device selection;
the browser reports transport status rather than live device health.
Run the copied command in a local
shell on the device or simulator computer. On Windows, use PowerShell or cmd.exe
with OpenSSH and WebMux on PATH; the receiving host needs an SSH server and must
resolve `webmux` in its noninteractive shell.

The default exporter port is 7575 for VirtualHere. Generic USB/IP commonly uses
3240; configure the actual port and, if required by the native importer, use
`--listen-port 3240` for the destination listener. Confirm the importer can use a
loopback endpoint and that the selected destination port is free. Backend
installation may expose additional listeners; secure those independently.

## Evaluate with a device or simulator and receiving computer

1. Build/install the WebMux version containing this command on **both computers**.
   The receiving computer must find `webmux` on the noninteractive SSH PATH.
   A development build (`cd webmux/server && go build -o /your/bin/webmux ./cmd/webmux`)
   can run this subcommand without frontend assets.
2. Configure native USB exporter/importer software independently. On the device
   or simulator computer, permit only the intended device and restrict the
   exporter's listener to loopback (or otherwise deny network access using its
   supported configuration).
   Do not expose the raw exporter to the Internet. The helper always dials
   `127.0.0.1`, but cannot restrict other listeners that external software opens.
3. Establish and verify normal SSH key/agent access to the receiving computer. The
   helper refuses unknown or changed host keys and never asks for a password.
   User SSH aliases and jump hosts are supported; `--ssh-port` selects a custom
   port.
4. From a **local shell on the device or simulator computer**, run:

   ```sh
   webmux usb-forward --host developer@development-host --export-port 7575 --duration 30m
   ```

   `7575` is the usual VirtualHere server port; use the actual exporter port.
   `--identity /path/to/existing/key` selects an existing SSH key. No key is
   uploaded to WebMux. `--listen-port 17575` requests a fixed remote loopback port;
   otherwise a free port is chosen and printed. `webmux usb-forward --help`
   lists all options.
5. On the **receiving computer**, point the native USB importer at the printed
   `127.0.0.1:PORT` address and explicitly select the intended device. For a
   VirtualHere evaluation, use its client UI to specify the hub address and
   select the device. Do not enable automatic device claiming.
6. Verify that the receiving OS enumerates the intended device and that a small
   known operation succeeds. For acceptance, the simulated mass-storage gadget
   below provides exact reads/writes without physical equipment. For a real
   peripheral, use its native application to check the operation you need.
7. Release the device in the native importer, then press Ctrl-C in the local
   forwarding command. The lease also expires automatically (default 30 minutes,
   maximum one hour). Plan the lease to cover the operation; expiry closes active
   channels. Unplug/replug or a dropped SSH connection does not authorize an
   automatic reconnect or claim of a different device.

### Browser status and Stop

The dialog's generated command includes an optional `--dashboard-link` argument.
The plain command in the example above remains usable with local console status
and Ctrl-C. To monitor and stop a lease from the browser, create its command in
the dialog and run that command within five minutes. Each pairing is consumed
once and is bound to the selected host, SSH port, exporter port and duration.

The native helper receives a temporary credential for reporting that lease; it
receives no WebMux account token. The browser lists and stops only its owner's
leases. Pairing and reporting credential digests are retained in the server's
private configuration so a replacement controller can recognize the running
helper and honor a previously requested stop.

**Transport ready** means that the SSH worker reported a receiving loopback
listener. Select the device in the native importer to verify attachment and
application behavior. **Stop requested** remains visible until the helper has
closed forwarding and acknowledged the remote worker's exit. Missing reports
show unavailable status; an expired lease or uncertain cleanup never claims
that the device was released. Closing the dialog leaves the foreground lease
running; reopen it, use Ctrl-C locally, or wait for the lease deadline to end it.

The helper's existing forwarding and SSH-cleanup deadlines remain in force.
Reporting uses bounded requests to the dashboard origin. HTTPS uses normal
certificate verification; links generated by an HTTP dashboard explicitly
permit HTTP. Redirects are refused so pairing/reporting credentials stay with
that origin. A lost dashboard connection leaves the local lease deadline and
Ctrl-C available; revoked reporting authority ends the helper's forwarding.

## Boundaries and lifecycle

- The transport grants the destination machine access to **every device shared
  by that exporter**. Device selection/allowlisting belongs to the native
  exporter. This is not a device-specific WebMux authorization boundary.
- The receiving computer's loopback listener is reachable by other local processes.
  Use a trusted receiving computer, with backend-native authentication where
  available. This is not isolation between mutually untrusted OS users.
- Existing SSH verifies the remote host and authenticates the local operator.
  A dedicated stdio channel carries a bounded binary protocol. No agent, X11,
  configured SSH forwards, PTY, local SSH commands or shared control connection
  are enabled. Remote execution is a fixed `webmux --usb-forward-worker` command.
- The worker binds `127.0.0.1` itself, so an `sshd` `GatewayPorts` setting cannot
  turn it into a public listener. The peer can only connect to the one fixed
  local exporter port; it cannot submit arbitrary hosts, ports or shell commands.
- At most eight TCP streams, 32 KiB frames, socket backpressure and a 15-second
  blocked-write deadline bound resources. Each side enforces the lease and
  closes the listener, sockets and transport on exit. After cancellation or
  expiry, the CLI closes relay input while continuing to drain SSH output, and
  waits up to three seconds for SSH to acknowledge remote-command completion before reporting released connections.
  If that fails, it stops its dedicated SSH child (with up to three more seconds
  to join stdio copiers), exits with an error and says remote release was not
  confirmed. An unreachable worker still has its own lease deadline; check the
  execution host before assuming its listener has disappeared. No USB payloads,
  device identifiers or native device-pairing records are logged or saved by
  the relay helper.
- A reachable exporter and a ready transport are distinct from a driver attaching
  the device. SSH stderr reports prerequisite failures; the helper never labels
  an opaque TCP connection as a successfully attached or usable USB device.

## Verification and remaining work

`go test -race ./internal/usbforward` exercises binary payloads larger than a
frame, half-close/reply behavior, concurrent stream isolation, connection caps,
exporter loss, cancellation, expiry, listener cleanup, invalid options,
oversized/malformed frames, occupied ports, and protocol violations. It also
uses a real OpenSSH client with ephemeral host/client keys against an in-process
SSH server, proves byte transfer through the fixed worker command, and checks
that an unknown host key is rejected. The fixture builds the WebMux executable,
executes a separate worker through SSH, and runs the real CLI against a dummy TCP
exporter, checking binary payloads and listener cleanup after lease expiry.
Gated remote-EOF controls keep the listener live after local cancellation and
verify that successful completion waits for remote cleanup, that a hung worker
fails within the cleanup bound, and that a failed remote exit never reports
released connections. The SSH fixture supplies remote exit status and leaves the transport open for
the client's disconnect after closing the session channel, matching sshd. A
completed-worker control verifies that this connection remains usable. A built
CLI failure control retains the cleanup explanation alongside SSH's nonzero
exit code; native coding-CLI aliases retain their original diagnostic behavior. These controls reproduce the premature-success ordering
reported in [#320](https://github.com/jordanhubbard/webmux/issues/320); its separate
federation and RDP reconnect observations remain open.
`--known-hosts` selects an explicit verified trust file for isolated environments;
strict host-key verification remains enabled. Keys and test traffic are disposable
fixtures, not host or device credentials.

Device acceptance uses the simulated USB tier below: enumerate the intended
device, read/write known bytes, explicitly reattach, interrupt forwarding and
verify device release plus worker/listener cleanup. Physical equipment and
device-specific workflows are optional follow-on checks. Browser status and
control integration is implemented under #485. Native acceptance additionally
checks one-use pairing, controller-loss continuity and dashboard Stop while
retaining the ordinary CLI interruption check. Keep #120 open until the
candidate's qualification and normal repository gates have evidence.


### Simulated USB device acceptance

The fake exporter is intentional: WebMux carries opaque TCP, so ordinary CI must
not require physical hardware or a USB driver. The built CLI/worker test validates
that boundary.
A second Linux-only CI tier runs `tests/usb-kernel.py` in a disposable Ubuntu VM.
It boots a generic kernel with matching modules in a nested, network-isolated VM
(hosted Azure kernels omit gadget drivers), records that kernel version,
creates a mass-storage gadget on the `usbip-vudc` virtual controller, and connects the native USB/IP exporter
to the `vhci_hcd` importer through the built WebMux CLI and a separate SSH worker.
It verifies enumeration, exact reads and writes, explicit detach, device
disappearance after interruption, and process/listener cleanup. Missing kernel
modules fail the dedicated job rather than silently skipping assertions. The
fixture refuses machines with existing USB forwarding modules and selects only
its own gadget; do not run it on a workstation with real USB devices.

Only a passing dedicated CI run establishes native-tier evidence. The guest
kernel is pinned in CI and recorded in its log, not a promise of compatibility
with every kernel. This simple simulated device is sufficient for #120's device
acceptance; it does not qualify every native backend, OS pair or peripheral.
See the [kernel gadget API](https://www.kernel.org/doc/html/latest/driver-api/usb/gadget.html)
and [USB/IP setup](https://github.com/torvalds/linux/blob/master/tools/usb/usbip/README).

An existing [main-source qualification run](https://github.com/jordanhubbard/webmux/actions/runs/44125271864/job/134187227229)
passed this dedicated tier at `8f5d3225fac65804554a65bc08381a03a70654fe`.
This satisfies the revised simulated-device requirement for that source; later
changes still need their own qualification and normal repository gates.
That earlier receipt covers the unmonitored relay. The dashboard/controller-loss
extension requires a new exact-source native run.

### Virtual gadget teardown qualification (#382)

The Ubuntu 6.8.0-146 virtual gadget can retain completed USB/IP replies after its
transmit thread exits. The next importer may receive an old URB reply, either
before its import handshake (reported as a protocol-version mismatch) or after
attachment (an unknown URB sequence number). This was reproduced in a direct
USB/IP control that launched no WebMux or SSH process.

CI uses `scripts/build-test-vudc.py` to build the virtual gadget from pinned Linux
v6.8 source against the selected guest kernel's headers. It verifies every source
hash and applies `tests/usb-vudc.patch` without fuzz to discard completed replies
during teardown. A shared baseline patch, `tests/usb-vudc-dequeue.patch`, also
obtains the controller from the endpoint when a request is dequeued before its
first queue; the original request field is uninitialized and caused a real
kernel NULL dereference during the repeated direct control. Both sides of the
stale-reply comparison include this endpoint correction. Kernel BUG/Oops/panic
output fails qualification even if a userspace command returns success.
The GPL-2.0-or-later module is a disposable test device; it is not
linked into or shipped with WebMux. The generic guest kernel, source commit,
patches, module digests and vermagic remain in the build receipts.

`scripts/qualify-test-vudc.py` checks three module builds in fresh isolated VMs:

- Identical source without the correction must reproduce the specific failure.
  A test-only scheduling hook pauses transmission of a real completed native
  read, then the fixture detaches the importer. Native TX stays held until the
  real replacement attach command acknowledges its import handshake. The fixture
  verifies that the hold is still active before releasing it, isolating stale
  delivery to the new native URB receiver. It creates no synthetic USB/IP requests
  or replies. The baseline must report the unknown URB and fail enumeration;
  handshake errors, setup failures, outer observation timeouts and unrelated errors
  do not qualify. Ordered native hold/detach/import/release records are required
  on both sides of the comparison. Existing deadlines are unchanged.
- The corrected source with the same hook must actually discard the held reply,
  reimport the device and read the exact expected bytes.
- The corrected source without the hook must pass 20 direct import/read/write/
  detach cycles, followed by the WebMux-over-SSH lifecycle in another fresh VM.
  That lifecycle includes explicit reattach, actual dashboard-controller loss,
  continued USB payload access, browser-owner Stop and acknowledged worker/
  listener cleanup. A separate ordinary CLI run retains the native SIGINT
  interruption and USB release checks.

Each VM compares the loaded module's source identity with the build receipt.
Logs, admission results and build receipts are retained. This qualifies WebMux's
opaque relay with the corrected virtual device; it does not qualify the stock
buggy virtual gadget, every Linux kernel, or every physical peripheral/backend.
