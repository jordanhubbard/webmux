#!/usr/bin/env python3
"""Disposable GitHub Linux VM only: real USB/IP over the shipped SSH relay.

No physical device is selected. A USB/IP virtual controller owns a temporary mass
storage gadget, and the importer must enumerate it below vhci_hcd.
"""
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

from usb_sysfs import imported_disk


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


if os.geteuid() != 0 or os.environ.get("GITHUB_ACTIONS") != "true" or socket.gethostname() != "virtme-ng":
    sys.exit("Run only as root in a disposable GitHub Actions Linux VM.")
binary = str(Path(sys.argv[1]).resolve(strict=True))
direct_control = sys.argv[2:] == ["--direct-control"]
stale_control = sys.argv[2:] == ["--stale-control"]
if sys.argv[2:] and not (direct_control or stale_control):
    sys.exit("Expected an optional --direct-control or --stale-control flag.")
modules = ["usbip_vudc", "g_mass_storage", "vhci_hcd"]
if any(Path("/sys/module", module).exists() for module in modules):
    sys.exit("Fixture requires a fresh VM with no existing USB forwarding modules.")
if any(listening(port) for port in [23422, 3241, 3242]):
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
        disk = until(imported_disk, "initial stale-control enumeration")
        with disk.open("rb", buffering=0) as source:
            assert source.read(len(pattern)) == pattern, "initial control payload mismatch"
        parameters = Path("/sys/module/usbip_vudc/parameters")
        held = parameters / "webmux_test_held_replies"
        before = int(held.read_text())
        (parameters / "webmux_test_hold_replies").write_text("1")
        reader = start("held-read", "dd", f"if={disk}", "of=/dev/null", "bs=4096", "skip=256", "count=1", "iflag=direct", "status=none")
        until(lambda: int(held.read_text()) > before, "real native completion queued under TX hold")
        assert reader.poll() is None, "direct read completed instead of remaining held"
        print("Genuine native reply queued; direct read is blocked", flush=True)
        ports = re.findall(r"Port (\d+):", run("usbip", "port"))
        assert len(ports) == 1, "Expected exactly one control import"
        run("usbip", "detach", "--port", ports[0])
        until(lambda: imported_disk() is None, "held-reply USB detach")
        assert reader.wait(timeout=10) != 0, "held read must be interrupted by detach"
        (parameters / "webmux_test_hold_replies").write_text("0")
        print("HELD REPLY REATTACH: beginning replacement import", flush=True)
        run("usbip", "--tcp-port", "3241", "attach", "--remote", "127.0.0.1", "--busid", busid)
        disk = until(imported_disk, "USB import after held-reply disconnect")
        with disk.open("rb", buffering=0) as source:
            assert source.read(len(pattern)) == pattern, "replacement control payload mismatch"
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
            disk = until(imported_disk, f"direct USB mass storage enumeration, cycle {cycle}")
            with disk.open("rb", buffering=0) as source:
                assert source.read(len(pattern)) == pattern, "direct USB read payload mismatch"
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
    cli = start("webmux", binary, "usb-forward", "--host", f"{account.pw_name}@127.0.0.1",
                "--ssh-port", "23422", "--identity", str(root / "identity"),
                "--known-hosts", str(root / "known_hosts"), "--export-port", "3241",
                "--listen-port", "3242", "--duration", "3m")
    def cli_ready():
        if cli.poll() is not None:
            raise RuntimeError("WebMux CLI exited before readiness; see captured SSH/CLI logs")
        return listening(3242)
    until(cli_ready, "WebMux SSH worker readiness")
    run("usbip", "--tcp-port", "3242", "list", "--remote", "127.0.0.1")

    def attach():
        run("usbip", "--tcp-port", "3242", "attach", "--remote", "127.0.0.1", "--busid", busid)
        disk = until(imported_disk, "USB mass storage enumeration through WebMux")
        print(f"Imported fixture device: {disk}", flush=True)
        with disk.open("rb", buffering=0) as source:
            assert source.read(len(pattern)) == pattern, "USB read payload mismatch"
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
    attach()
    cli.send_signal(signal.SIGINT)
    assert cli.wait(timeout=20) == 0, "CLI cancellation failed"
    until(lambda: imported_disk() is None, "USB detach after relay interruption")
    until(lambda: not listening(3242), "receiving listener teardown")
    worker_pid = int((root / "worker.pid").read_text())
    until(lambda: not Path(f"/proc/{worker_pid}").exists(), "SSH worker process teardown")
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
