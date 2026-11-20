#!/usr/bin/env python3
"""Disposable Linux VM only: real USB/IP over the shipped SSH relay.

No physical device is selected. A USB/IP virtual controller owns a temporary mass
storage gadget, and the importer must enumerate it below vhci_hcd.
"""
import base64
import json
import os
from pathlib import Path
import pwd
import re
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request

from usb_sysfs import imported_disk, imported_disk_with_payload
from usb_hold_diagnostics import held_reply_diagnostic


def run(*args, check=True):
    result = subprocess.run(args, text=True, stdout=subprocess.PIPE,
                            stderr=subprocess.STDOUT, timeout=45, check=False)
    print("+", *args, flush=True)
    print(result.stdout, end="", flush=True)
    if check and result.returncode:
        raise RuntimeError(f"command failed ({result.returncode}): {args[0]}")
    return result.stdout


def until(check, description, seconds=30):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        result = check()
        if result:
            return result
        time.sleep(0.2)
    raise RuntimeError(f"Timed out: {description}")


def listening(port):
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=0.2):
            return True
    except OSError:
        return False


local_parent = os.environ.get('WEBMUX_USB_LOCAL_PARENT_BOOT_ID')
if local_parent is not None:
    sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'scripts'))
    from usb_fixture_context import local_guest
    try:
        local_guest(local_parent)
    except (OSError, ValueError) as error:
        sys.exit(str(error))
if os.geteuid() != 0 or (local_parent is None and os.environ.get("GITHUB_ACTIONS") != "true") or socket.gethostname() != "virtme-ng":
    sys.exit("Run only as root in a disposable hosted or admitted local virtme Linux VM.")
binary = str(Path(sys.argv[1]).resolve(strict=True))
direct_control = sys.argv[2:] == ["--direct-control"]
stale_control = sys.argv[2:] in [["--stale-control"], ["--prequeue-control"]]
prequeue_control = sys.argv[2:] == ["--prequeue-control"]
if sys.argv[2:] and not (direct_control or stale_control):
    sys.exit("Expected an optional --direct-control, --stale-control or --prequeue-control flag.")
modules = ["usbip_vudc", "g_mass_storage", "vhci_hcd"]
if any(Path("/sys/module", module).exists() for module in modules):
    sys.exit("Fixture requires a fresh VM with no existing USB forwarding modules.")
if any(listening(port) for port in [23422, 23423, 3241, 3242]):
    sys.exit("Fixture ports are already in use.")
# Run the key-only fixture sshd unprivileged, as in the real SSH browser tests.
# It cannot read host shadow data or launch a worker as a different user.
account = pwd.getpwnam("runner")
root = Path(tempfile.mkdtemp(prefix="webmux-usb-kernel-"))
root.chmod(0o755)
os.chown(root, account.pw_uid, account.pw_gid)
children = []
logs = []
loaded = []
busid = None


def start(name, *args, user=None, group=None):
    log = open(root / f"{name}.log", "w+")
    logs.append((name, log))
    child = subprocess.Popen(args, stdout=log, stderr=subprocess.STDOUT,
                             stdin=subprocess.DEVNULL, start_new_session=True,
                             user=user, group=group, extra_groups=[] if user is not None else None)
    children.append(child)
    return child



try:
    run("uname", "-a")
    run("ip", "link", "set", "lo", "up")
    run("usbip", "version")
    for module in ["usbip_vudc", "vhci_hcd"]:
        run("modprobe", module)
        loaded.append(module)
    expected_module = os.environ.get("WEBMUX_USB_EXPECTED_SRCVERSION")
    if expected_module:
        actual_module = Path("/sys/module/usbip_vudc/srcversion").read_text().strip()
        assert actual_module == expected_module, "loaded virtual gadget identity mismatch"
        print(f"Loaded gadget srcversion verified: {actual_module}", flush=True)
    image = root / "storage.img"
    pattern = bytes(range(256)) * 16
    with image.open("wb") as output:
        output.write(pattern)
        output.truncate(32 * 1024 * 1024)
    run("modprobe", "g_mass_storage", f"file={image}", "removable=1", "stall=0", "iSerialNumber=webmux-ci-storage")
    loaded.append("g_mass_storage")

    def gadget_bus():
        for entry in Path("/sys/class/udc").glob("usbip-vudc.*"):
            if (entry / "function").read_text().strip():
                return entry.name
        return None

    busid = until(gadget_bus, "virtual USB gadget binding")
    start("usbipd", "usbipd", "--device", "-4", "--tcp-port", "3241")
    until(lambda: listening(3241), "USB/IP exporter startup")

    if stale_control:
        print("STALE REPLY CONTROL: real native completion held before disconnect", flush=True)
        run("usbip", "--tcp-port", "3241", "attach", "--remote", "127.0.0.1", "--busid", busid)
        disk = until(lambda: imported_disk_with_payload(pattern), "initial stale-control enumeration")
        parameters = Path("/sys/module/usbip_vudc/parameters")
        held = parameters / "webmux_test_held_replies"
        before = int(held.read_text())
        if prequeue_control:
            # Stop TX first, leaving the original hold/counter disabled. One
            # real completion can block the next SCSI stage; no reply is made
            # up and the counter must still equal its original baseline.
            paused = parameters / "webmux_test_pause_tx"
            pending = parameters / "webmux_test_prequeued_replies"
            paused.write_text("1")
            reader = start("held-read", "dd", f"if={disk}", "of=/dev/null", "bs=4096", "skip=256", "count=1", "iflag=direct", "status=none")
            until(lambda: int(pending.read_text()) > 0, "genuine native completion queued before hold")
            assert reader.poll() is None
            assert int(held.read_text()) == before, "hold counter changed while hold was disabled"
            print("PREQUEUE_CONTROL " + json.dumps({"queued": int(pending.read_text()), "before": before, "readerBlocked": True}), flush=True)
            (parameters / "webmux_test_hold_replies").write_text("1")
            paused.write_text("0")
        else:
            (parameters / "webmux_test_hold_replies").write_text("1")
            reader = start("held-read", "dd", f"if={disk}", "of=/dev/null", "bs=4096", "skip=256", "count=1", "iflag=direct", "status=none")
        try:
            until(lambda: int(held.read_text()) > before, "real native completion queued under TX hold")
        except RuntimeError:
            print('HELD_REPLY_DIAGNOSTIC ' + json.dumps(
                held_reply_diagnostic(parameters, before, reader, disk)), flush=True)
            raise
        assert reader.poll() is None, "direct read completed instead of remaining held"
        if prequeue_control:
            def counted_queue():
                return int(held.read_text()) - before == int(pending.read_text()) > 0
            until(counted_queue, "held counter matches the actual native completion queue")
            time.sleep(0.05)  # Multiple unchanged 10ms TX hold iterations.
            until(counted_queue, "held counter still matches the actual native completion queue")
            assert reader.poll() is None and paused.read_text().strip() in {"N", "0"}
            print("PREQUEUE_COUNTER " + json.dumps({"held": int(held.read_text()) - before, "queued": int(pending.read_text())}), flush=True)
        print("Genuine native reply queued; direct read is blocked", flush=True)
        ports = re.findall(r"Port (\d+):", run("usbip", "port"))
        assert len(ports) == 1, "Expected exactly one control import"
        run("usbip", "detach", "--port", ports[0])
        until(lambda: imported_disk() is None, "held-reply USB detach")
        assert reader.wait(timeout=10) != 0, "held read must be interrupted by detach"
        print("Held native read interrupted by detach", flush=True)
        print("HELD REPLY REATTACH: beginning replacement import", flush=True)
        # Keep native data replies held while usbipd completes the userspace
        # import handshake. The kernel's stale completion must be observed by
        # the new native URB receiver, not interleaved with its control header.
        run("usbip", "--tcp-port", "3241", "attach", "--remote", "127.0.0.1", "--busid", busid)
        assert (parameters / "webmux_test_hold_replies").read_text().strip() in {"Y", "1"}, "native TX hold ended before import acknowledgment"
        print("Replacement import acknowledged while native TX remained held", flush=True)
        print("Releasing held native replies after import acknowledgment", flush=True)
        (parameters / "webmux_test_hold_replies").write_text("0")
        disk = until(lambda: imported_disk_with_payload(pattern), "USB import after held-reply disconnect")
        kernel_log = run("dmesg")
        assert re.search(r"discarded [1-9][0-9]* stale USB/IP replies", kernel_log), "correction did not discard the held completion"
        print("PASS: held native reply discarded; replacement import and exact payload verified", flush=True)
        sys.exit(0)

    if direct_control:
        # The same virtual gadget, native importer and kernel are used, but no
        # WebMux CLI, worker or SSH transport exists in this control.
        print("DIRECT USB/IP CONTROL: bypassing WebMux and SSH", flush=True)
        for cycle in range(1, 21):
            run("usbip", "--tcp-port", "3241", "attach", "--remote", "127.0.0.1", "--busid", busid)
            disk = until(lambda: imported_disk_with_payload(pattern), f"direct USB mass storage enumeration, cycle {cycle}")
            changed = bytes([cycle]) * len(pattern)
            with disk.open("r+b", buffering=0) as device:
                device.seek(1024 * 1024)
                device.write(changed)
                os.fsync(device.fileno())
            with image.open("rb", buffering=0) as backing:
                backing.seek(1024 * 1024)
                assert backing.read(len(changed)) == changed, "direct USB write payload mismatch"
            ports = re.findall(r"Port (\d+):", run("usbip", "port"))
            assert len(ports) == 1, "Expected exactly one direct imported fixture device"
            run("usbip", "detach", "--port", ports[0])
            until(lambda: imported_disk() is None, f"direct USB detach, cycle {cycle}")
            print(f"Direct cycle {cycle}/20 passed", flush=True)
        print("PASS: 20 direct USB/IP enumeration, read/write and detach cycles", flush=True)
        sys.exit(0)

    run("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", str(root / "host"))
    os.chown(root / "host", account.pw_uid, account.pw_gid)
    run("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", str(root / "identity"))
    (root / "known_hosts").write_text("[127.0.0.1]:23422 " + (root / "host.pub").read_text())
    # Only this generated key can invoke the fixed worker command.
    wrapper = root / "worker.sh"
    import shlex
    wrapper.write_text(f"#!/bin/sh\necho $$ > {shlex.quote(str(root / 'worker.pid'))}\nexec {shlex.quote(binary)} --usb-forward-worker\n")
    wrapper.chmod(0o755)
    config = root / "sshd_config"
    config.write_text(f"""Port 23422
ListenAddress 127.0.0.1
HostKey {root}/host
PidFile {root}/sshd.pid
AuthorizedKeysFile {root}/identity.pub
StrictModes no
PasswordAuthentication no
KbdInteractiveAuthentication no
UsePAM no
PermitRootLogin no
AllowUsers {account.pw_name}
ForceCommand {wrapper}
AllowTcpForwarding no
X11Forwarding no
""")
    Path("/run/sshd").mkdir(exist_ok=True)
    start("sshd", "/usr/sbin/sshd", "-D", "-e", "-f", str(config),
          user=account.pw_uid, group=account.pw_gid)
    until(lambda: listening(23422), "fixture SSH server startup")
    # The actual WebMux controller owns the lease, independently of the
    # foreground data helper and its SSH worker. No physical USB is selected.
    dashboard_home = root / "dashboard"
    (dashboard_home / "config").mkdir(parents=True)
    (dashboard_home / "config" / "auth.yaml").write_text("auth:\n  mode: none\n  users: []\n")
    defaults = Path(__file__).resolve().parents[1] / "webmux"
    app = (defaults / "config.defaults" / "app.yaml").read_text()
    app = app.replace("listen_host: 0.0.0.0", "listen_host: 127.0.0.1")
    (dashboard_home / "config" / "app.yaml").write_text(app)
    for entry in [dashboard_home, dashboard_home / "config", *list((dashboard_home / "config").iterdir())]:
        os.chown(entry, account.pw_uid, account.pw_gid)

    def start_dashboard(name):
        return start(name, "env", "WEBMUX_COORDINATION_DISABLED=1", binary,
                     "--root", str(defaults), "--home", str(dashboard_home),
                     "--listen", "127.0.0.1:23423", user=account.pw_uid, group=account.pw_gid)

    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    def dashboard_api(path, data=None):
        body = None if data is None else json.dumps(data).encode()
        request = urllib.request.Request("http://127.0.0.1:23423/api/" + path, data=body,
                                         headers={"Content-Type": "application/json"})
        with opener.open(request, timeout=3) as response:
            return json.load(response)

    def dashboard_ready():
        try:
            return dashboard_api("health").get("status") == "ok"
        except (OSError, ValueError):
            return False

    controller = start_dashboard("dashboard")
    until(dashboard_ready, "actual WebMux dashboard readiness")
    ticket = dashboard_api("usb-forwardings", {"host": f"{account.pw_name}@127.0.0.1",
                          "ssh_port": 23422, "export_port": 3241, "seconds": 180})
    lease_id = ticket["lease"]["id"]
    link = base64.urlsafe_b64encode(json.dumps({"version": 1, "server": "http://127.0.0.1:23423",
             "id": lease_id, "pair_code": ticket["pair_code"], "allow_http": True}).encode()).decode().rstrip("=")
    cli = start("webmux", binary, "usb-forward", "--host", f"{account.pw_name}@127.0.0.1",
                "--ssh-port", "23422", "--identity", str(root / "identity"),
                "--known-hosts", str(root / "known_hosts"), "--export-port", "3241",
                "--listen-port", "3242", "--duration", "3m", "--dashboard-link", link)
    def cli_ready():
        if cli.poll() is not None:
            raise RuntimeError("WebMux CLI exited before readiness; see captured SSH/CLI logs")
        return listening(3242)
    until(cli_ready, "WebMux SSH worker readiness")
    def current_lease():
        values = dashboard_api("usb-forwardings")
        return next(value for value in values if value["id"] == lease_id)

    until(lambda: current_lease()["phase"] == "ready", "native helper dashboard report")

    run("usbip", "--tcp-port", "3242", "list", "--remote", "127.0.0.1")

    def attach():
        run("usbip", "--tcp-port", "3242", "attach", "--remote", "127.0.0.1", "--busid", busid)
        disk = until(lambda: imported_disk_with_payload(pattern), "USB mass storage enumeration through WebMux")
        print(f"Imported fixture device with exact payload verified: {disk}", flush=True)
        return disk

    disk = attach()
    changed = bytes(reversed(range(256))) * 16
    with disk.open("r+b", buffering=0) as device:
        device.seek(1024 * 1024)
        device.write(changed)
        os.fsync(device.fileno())
    with image.open("rb", buffering=0) as backing:
        backing.seek(1024 * 1024)
        assert backing.read(len(changed)) == changed, "USB write payload mismatch"
    ports = re.findall(r"Port (\d+):", run("usbip", "port"))
    assert len(ports) == 1, "Expected exactly one imported fixture device"
    run("usbip", "detach", "--port", ports[0])
    until(lambda: imported_disk() is None, "explicit USB detach")
    disk = attach()
    controller.kill()
    assert controller.wait(timeout=5) != 0, "owned controller crash was not observed"
    until(lambda: not listening(23423), "crashed dashboard listener teardown")
    controller = start_dashboard("dashboard-replacement")
    until(dashboard_ready, "replacement dashboard readiness")
    with disk.open("rb", buffering=0) as source:
        assert source.read(len(pattern)) == pattern, "USB data did not survive controller loss"
    stop = dashboard_api("usb-forwardings/" + lease_id + "/stop", {})
    assert stop["phase"] == "stopping" and not stop["release_confirmed"], "Stop request claimed remote release"
    assert cli.wait(timeout=20) == 0, "dashboard Stop failed"
    ended = current_lease()
    assert ended["phase"] == "ended" and ended["release_confirmed"], "missing acknowledged remote release"

    until(lambda: imported_disk() is None, "USB detach after relay interruption")
    until(lambda: not listening(3242), "receiving listener teardown")
    worker_pid = int((root / "worker.pid").read_text())
    until(lambda: not Path(f"/proc/{worker_pid}").exists(), "SSH worker process teardown")
    controller.send_signal(signal.SIGTERM)
    assert controller.wait(timeout=10) == 0, "dashboard controller cleanup failed"
    until(lambda: not listening(23423), "dashboard listener cleanup")
    # Preserve the original signal-interruption acceptance using the ordinary
    # unmonitored CLI as well; dashboard Stop is an additional control.
    for port in re.findall(r"Port (\d+):", run("usbip", "port")):
        run("usbip", "detach", "--port", port)
    cli = start("webmux-legacy-interrupt", binary, "usb-forward", "--host", f"{account.pw_name}@127.0.0.1",
                "--ssh-port", "23422", "--identity", str(root / "identity"),
                "--known-hosts", str(root / "known_hosts"), "--export-port", "3241",
                "--listen-port", "3242", "--duration", "3m")
    until(cli_ready, "ordinary WebMux SSH worker readiness")
    attach()
    cli.send_signal(signal.SIGINT)
    assert cli.wait(timeout=20) == 0, "ordinary CLI signal cancellation failed"
    until(lambda: imported_disk() is None, "USB detach after ordinary CLI interruption")
    until(lambda: not listening(3242), "ordinary receiving listener teardown")
    worker_pid = int((root / "worker.pid").read_text())
    until(lambda: not Path(f"/proc/{worker_pid}").exists(), "ordinary SSH worker process teardown")
    print("PASS: dashboard pairing, controller-loss continuity, owner Stop and confirmed listener release", flush=True)
    print("PASS: virtual USB enumeration, read/write, detach, interruption and worker cleanup", flush=True)
finally:
    run("dmesg", check=False)
    for port in re.findall(r"Port (\d+):", run("usbip", "port", check=False)):
        run("usbip", "detach", "--port", port, check=False)
    for child in reversed(children):
        if child.poll() is None:
            os.killpg(child.pid, signal.SIGTERM)
            try:
                child.wait(timeout=5)
            except subprocess.TimeoutExpired:
                os.killpg(child.pid, signal.SIGKILL)
                child.wait(timeout=5)
    for module in reversed(loaded):
        run("modprobe", "-r", module, check=False)
    for name, log in logs:
        log.seek(0)
        print(f"--- {name} ---\n{log.read()}", flush=True)
        log.close()
    shutil.rmtree(root)
