# Experimental USB forwarding transport

Run `webmux usb-forward` on the **Mac beside the physical device** to carry a
native USB exporter's TCP traffic to the development Mac you use through WebMux.
The remote helper opens an IPv4 loopback listener; a native USB importer on that
Mac connects to it. This is a foreground CLI companion in the WebMux binary,
not a browser USB picker or an HTTP device-control API.

**Status:** transport implementation with synthetic TCP and real OpenSSH tests.
No physical USB backend, iPhone, Xcode discovery, install, or debug workflow has
been qualified. This does not yet establish that remote iPhone development works.
Track hardware acceptance and browser integration in
[#120](https://github.com/jordanhubbard/webmux/issues/120).

## Why native software is required

A browser does not attach devices to another machine's USB bus. Full device
forwarding needs an exporter beside the device and an importer/driver on the
machine running Xcode. WebMux supplies the authenticated transport between them;
it neither implements nor bundles those drivers.

For Mac-to-Mac evaluation, [VirtualHere](https://www.virtualhere.com/usb_client_software)
is an example of a native exporter/importer pair, not a qualified or bundled
WebMux dependency. Its [macOS server](https://www.virtualhere.com/osx_server_software)
and licensing must be obtained separately. Confirm that the selected versions
work with your macOS, iOS and Xcode before relying on them. The vendor has
[reported Apple-service conflicts for Mac-to-Mac iPhone use](https://www.virtualhere.com/node/4585).
WebMux does not stop `usbmuxd`, install system extensions, disable SIP, or change
system configuration to work around those conflicts.

[Apple documents](https://developer.apple.com/documentation/technotes/tn3158-resolving-xcode-15-device-connection-issues)
that modern Xcode device communication uses a network interface even over USB.
[libusbmuxd's alternative socket](https://github.com/libimobiledevice/libusbmuxd)
can support compatible device CLI tools, but is not evidence of CoreDevice/Xcode
compatibility. The open-source [macOS USB/IP client](https://github.com/carlossless/usbip-macos)
also documents a special entitlement requirement; its SIP-disabled fallback is
not part of this implementation.

## Evaluate with your Mac and development Mac

1. Build/install the WebMux version containing this command on **both Macs**.
   The development Mac must find `webmux` on the noninteractive SSH PATH.
   A development build (`cd webmux/server && go build -o /your/bin/webmux ./cmd/webmux`)
   can run this subcommand without frontend assets.
2. Configure native USB exporter/importer software independently. On the device
   Mac, permit only the intended iPhone and restrict the exporter's listener to
   loopback (or otherwise deny network access using its supported configuration).
   Do not expose the raw exporter to the Internet. The helper always dials
   `127.0.0.1`, but cannot restrict other listeners that external software opens.
3. Establish and verify normal SSH key/agent access to the development Mac. The
   helper refuses unknown or changed host keys and never asks for a password.
   User SSH aliases and jump hosts are supported; `--ssh-port` selects a custom
   port. Signing certificates and Apple account access must separately exist
   on the Mac that builds/signs the app.
4. From a **local shell on the Mac with the iPhone**, run:

   ```sh
   webmux usb-forward --host developer@build-mac --export-port 7575 --duration 30m
   ```

   `7575` is the usual VirtualHere server port; use the actual exporter port.
   `--identity /path/to/existing/key` selects an existing SSH key. No key is
   uploaded to WebMux. `--listen-port 17575` requests a fixed remote loopback port;
   otherwise a free port is chosen and printed. `webmux usb-forward --help`
   lists all options.
5. On the **development Mac**, point the native USB importer at the printed
   `127.0.0.1:PORT` address and explicitly select the intended device. For a
   VirtualHere evaluation, use its client UI to specify the hub address and
   select the device. Do not enable automatic device claiming.
6. Complete native trust/Developer Mode prompts yourself. Verify a **physical**
   device in Xcode/`devicectl`: recent Xcode versions also list simulated devices,
   so check `hardwareProperties.reality`, not just an iPhone model name.
   Then verify a signed app installation, launch and debug session before calling
   this arrangement usable. Preserve the installed app's bundle ID/team and any
   App Attest identity/policy.
7. Release the device in the native importer, then press Ctrl-C in the local
   forwarding command. The lease also expires automatically (default 30 minutes,
   maximum one hour). Plan the lease to cover the operation; expiry closes active
   channels. Unplug/replug or a dropped SSH connection does not authorize an
   automatic reconnect or claim of a different device.

## Boundaries and lifecycle

- The transport grants the destination machine access to **every device shared
  by that exporter**. Device selection/allowlisting belongs to the native
  exporter. This is not a device-specific WebMux authorization boundary.
- The development Mac's loopback listener is reachable by other local processes.
  Use a trusted development Mac, with backend-native authentication where
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
  closes the listener, sockets and transport on exit. No payloads, device
  identifiers or pairing records are logged or saved by the helper.
- A reachable exporter and a ready transport are distinct from a driver attaching
  the device. SSH stderr reports prerequisite failures; the helper never labels
  an opaque TCP connection as a healthy iPhone or successful Xcode session.

## Verification and remaining work

`go test -race ./internal/usbforward` exercises binary payloads larger than a
frame, half-close/reply behavior, concurrent stream isolation, connection caps,
exporter loss, cancellation, expiry, listener cleanup, invalid options,
oversized/malformed frames, occupied ports, and protocol violations. It also
uses a real OpenSSH client with ephemeral host/client keys against an in-process
SSH server, proves byte transfer through the fixed worker command, and checks
that an unknown host key is rejected. Keys and test traffic are disposable
fixtures, not host or phone credentials.

Still required: native exporter/importer qualification on both Macs, reconnect
and device release after interruption, actual iPhone/Xcode install/debug, and a
browser-assisted setup/status flow. No driver or system service has been
installed or modified as part of this feature. Keep issue #120 open until these
acceptance conditions have evidence.
