# RDP proxy setup and qualification

WebMux connects to a separately installed Apache Guacamole proxy (`guacd`).
Use guacd 1.6.0 or newer with its matching RDP plugin and native libraries.
Run `make check-guacd` on the proxy host to check the installed version. This
optional check warns without preventing unrelated terminals from starting.
For a remote proxy, check its version on that host.

Older distro packages are insufficient for reliable reconnects. In a real Linux
ARM64 browser test, guacd 1.3.0 loses the initial 1024×768 size when the RDP
thread sends it before the first user is registered. The initial blue frame
and subsequent green update pass, but the reload never gets a sized desktop.
Under the same ordering control, guacd 1.6.0 draws the green desktop after reload
and fullscreen reconnect, and delivers browser keys to the real X server.
The historical hosted AMD64 failure has the same missing-frame symptom; its
original logs do not prove the scheduling order.

Apache's [1.6.0 release](https://guacamole.apache.org/releases/1.6.0/) introduces
a new rendering pipeline and preserves compatibility with older 1.x protocol
components. WebMux's browser protocol package remains covered by the real
rendering and keyboard tests. Do not mix native plugins from different guacd
releases.

On macOS, install `guacamole-server` through Homebrew and check `guacd -v`.
On Linux, check the distro's version before installing. CI builds the pinned
1.6.0 source into a private prefix with `scripts/build-guacd.py`, verifies its
SHA-256 and Apache release signature against a pinned signer, and retains a
build receipt. The builder requires Linux, a C toolchain, pkg-config, curl,
GnuPG, Cairo/JPEG/PNG/UUID/OpenSSL development libraries and FreeRDP 2 development
files. For an equivalent private build:

```sh
python3 scripts/build-guacd.py --prefix "$HOME/guacd-1.6" --build-root "$HOME/guacd-build"
LD_LIBRARY_PATH="$HOME/guacd-1.6/lib" "$HOME/guacd-1.6/sbin/guacd" -f -b 127.0.0.1
```

Both directories must be new or empty. Configure WebMux's `app.guacd` host
and port to reach the proxy. Keep it bound to a trusted interface; the Guacamole
protocol does not provide the browser's WebMux authentication boundary.

CI also runs `scripts/qualify-rdp-startup.py` against the distro daemon and the
private build. Its Linux-only preload fixture changes thread scheduling, without
altering protocol messages. It gives the baseline connection a registration
head start, then holds the reconnect's registration until the initial size is
sent or a bounded wait expires. Admission requires actual blue/green pixels,
the specific legacy reload failure, and a successful qualified run through
keyboard delivery. A separate ordinary test keeps the previous viewer's real
upstream open while withholding acknowledgements; the new viewer must draw
before that upstream closes. Screenshots, per-connection frame-leading opcode counts and
bounded process/frame receipts are retained on failure. The deadlines and
frame-presence checks remain in place. PNG pixels and alpha are exact. When the
current connection actually carries a JPEG instruction, saturated fixture RGB
channels may differ by at most three 8-bit levels, matching lossy encoding rather
than accepting a missing or stale frame. Raw pixel samples and JPEG wire evidence
remain in the receipt; opaque black, the previous blue frame during a green
phase, incorrect alpha, and larger channel errors still fail.
