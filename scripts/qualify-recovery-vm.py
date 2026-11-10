#!/usr/bin/env python3
"""Qualify #318 in its disposable QEMU fixture, including an actual guest reboot.

Requires the fixture's private launch.json, SSH arguments and user-service
configuration. Never targets a workstation or a shared worker. Uses a synthetic
long-running job; native conversational CLI qualification is a separate harness.
"""

import argparse
import hashlib
import json
from pathlib import Path
import shlex
import socket
import subprocess
import time
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--vm-root", required=True, type=Path)
    parser.add_argument("--update-binary", type=Path, help="optional different Linux ARM64 standalone binary to install during the test")
    args = parser.parse_args()
    root = args.vm_root.resolve()
    launch = json.loads((root / "launch.json").read_text())
    ssh_args = json.loads((root / "ssh-argv.json").read_text())
    service = json.loads((root / "service-control.json").read_text())
    assert ssh_args[-1] == "webmuxqa@127.0.0.1" and launch["ssh_port"] == 32222
    assert service["scope"] == "user" and str(service["uid"]).isdigit()
    assert service["unit"] == "webmux-qualification.service"
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as connection:
        connection.settimeout(3)
        connection.connect(str(root / "qmp.sock"))
        stream = connection.makefile("rwb", buffering=0)
        json.loads(stream.readline())
        stream.write(b'{"execute":"qmp_capabilities"}\n')
        json.loads(stream.readline())
        stream.write(b'{"execute":"query-status"}\n')
        assert json.loads(stream.readline())["return"]["running"]
    process = subprocess.run(["ps", "-p", str(launch["pid"]), "-o", "command="], capture_output=True, text=True, check=True)
    assert "qemu-system-aarch64" in process.stdout and str(root / "disk.qcow2") in process.stdout
    base = "http://127.0.0.1:" + str(launch["http_port"])
    repo = Path(__file__).resolve().parents[1]
    uid = str(service["uid"])
    control = "env XDG_RUNTIME_DIR=/run/user/" + uid + " DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/" + uid + "/bus systemctl --user"
    stages = []
    result_file = root / ("local-lifecycle-" + str(uuid.uuid4()) + ".json")

    def ssh(command, check=True, timeout=12):
        result = subprocess.run(ssh_args[:-1] + ["-o", "ConnectTimeout=2", ssh_args[-1], command], capture_output=True, text=True, timeout=timeout)
        if check and result.returncode:
            raise RuntimeError("guest command failed: " + result.stderr[-500:])
        return result

    assert ssh("hostname").stdout.strip() == "webmux-recovery-qa"
    assert ssh("test -f /var/tmp/webmux-qualification-ready").returncode == 0

    def api(path, data=None, method=None):
        encoded = json.dumps(data).encode() if data is not None else None
        request = urllib.request.Request(base + path, data=encoded, method=method, headers={"Content-Type": "application/json"})
        with urllib.request.urlopen(request, timeout=5) as response:
            payload = response.read()
            return json.loads(payload) if payload else None

    def wait_for(predicate, label, seconds=75):
        deadline = time.monotonic() + seconds
        last = None
        while time.monotonic() < deadline:
            try:
                value = predicate()
                if value:
                    return value
            except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
                last = str(error)
            time.sleep(.25)
        raise RuntimeError("timed out waiting for " + label + ": " + str(last))

    api("/api/config", {"app": {"session_logging": {"enabled": True}}}, "PUT")
    session = api("/api/sessions", {"transport": "local", "cols": 100, "rows": 30})
    session_id = str(uuid.UUID(session["id"]))
    target = "webmux-" + session_id
    count_path = "/home/webmuxqa/qualification/executions-" + session_id
    marker = "VM_RECOVERY_" + session_id
    recovery_path = "/api/session-recovery/" + session_id

    def executions():
        value = ssh("test ! -f " + shlex.quote(count_path) + " || wc -l < " + shlex.quote(count_path)).stdout.strip()
        return int(value or "0")

    def identities():
        value = ssh("tmux display-message -p -t " + target + " '#{pid}:#{pane_pid}'").stdout.strip()
        provider, pane = value.split(":")
        return int(provider), int(pane)

    def boot_id():
        return str(uuid.UUID(ssh("cat /proc/sys/kernel/random/boot_id").stdout.strip()))

    def service_pid():
        return int(ssh(control + " show " + service["unit"] + " -p MainPID --value").stdout.strip())

    def record(name):
        view = api(recovery_path)
        provider, pane = identities()
        item = {"stage": name, "boot_id": boot_id(), "server_pid": service_pid(), "provider_pid": provider, "pane_pid": pane,
                "executions": executions(), "waiting": view["waiting"], "transcripts": view.get("state", {}).get("transcripts", [])}
        stages.append(item)
        result_file.write_text(json.dumps({"session_id": session_id, "stages": stages}, indent=2) + "\n")
        result_file.chmod(0o600)
        print(json.dumps(item), flush=True)
        return item

    try:
        wait_for(lambda: api(recovery_path)["shell_ready"], "initial shell readiness")
        code = "require('node:fs').appendFileSync(" + json.dumps(count_path) + ", 'execution\\n'); console.log(" + json.dumps(marker) + "); setInterval(() => console.log('qa tick'), 1000)"
        command = "/usr/bin/node -e " + shlex.quote(code)
        bridge = """import fs from 'node:fs'; import WebSocket from 'ws';
const input=JSON.parse(fs.readFileSync(0,'utf8')); const ws=new WebSocket(input.url);
const timer=setTimeout(()=>{ws.terminate();process.exitCode=1},5000);
ws.on('error',()=>{clearTimeout(timer);process.exitCode=1});
ws.on('open',()=>{ws.send(JSON.stringify({type:'input',data:input.command+'\\r'}));setTimeout(()=>{clearTimeout(timer);ws.close()},500)});
"""
        subprocess.run(["node", "--input-type=module", "-e", bridge], cwd=repo / "webmux", input=json.dumps({"url": base.replace("http:", "ws:") + "/api/term/" + session_id, "command": command}), text=True, check=True, timeout=10)
        wait_for(lambda: executions() == 1, "initial job")
        wait_for(lambda: marker in api(recovery_path)["history"], "persisted pre-interruption history")
        original = record("running")
        group = ssh("cat /proc/" + str(original["provider_pid"]) + "/cgroup").stdout
        assert ".scope" in group and "webmux-qualification.service" not in group

        ssh(control + " restart " + service["unit"])
        wait_for(lambda: api("/api/health")["status"] == "ok" and service_pid() != original["server_pid"], "orderly service restart")
        wait_for(lambda: api(recovery_path).get("state", {}).get("disposition") == "reattached", "surviving provider reattachment")
        restarted = record("orderly-restart")
        assert restarted["provider_pid"] == original["provider_pid"] and restarted["pane_pid"] == original["pane_pid"] and restarted["executions"] == 1

        if args.update_binary:
            binary = args.update_binary.resolve()
            digest = hashlib.sha256(binary.read_bytes()).hexdigest()
            previous = ssh("sha256sum /usr/local/bin/webmux").stdout.split()[0]
            assert previous != digest, "update must install a different standalone executable"
            subprocess.run(ssh_args + ["cat > /home/webmuxqa/qualification/webmux-update"], input=binary.read_bytes(), check=True, timeout=30)
            ssh("chmod 700 /home/webmuxqa/qualification/webmux-update; sudo -n install -m 755 /home/webmuxqa/qualification/webmux-update /usr/local/bin/webmux.next; sudo -n mv -f /usr/local/bin/webmux.next /usr/local/bin/webmux; " + control + " restart " + service["unit"])
            wait_for(lambda: api("/api/health")["status"] == "ok" and service_pid() != restarted["server_pid"], "updated service")
            assert ssh("sha256sum /usr/local/bin/webmux").stdout.split()[0] == digest
            updated = record("binary-update")
            assert updated["provider_pid"] == original["provider_pid"] and updated["pane_pid"] == original["pane_pid"] and updated["executions"] == 1

        before_crash = service_pid()
        ssh("kill -KILL " + str(before_crash))
        wait_for(lambda: api("/api/health")["status"] == "ok" and service_pid() != before_crash, "restart after SIGKILL")
        wait_for(lambda: api(recovery_path).get("state", {}).get("disposition") == "reattached", "reattachment after SIGKILL")
        crashed = record("abrupt-termination")
        assert crashed["provider_pid"] == original["provider_pid"] and crashed["pane_pid"] == original["pane_pid"] and crashed["executions"] == 1

        prior_boot = boot_id()
        reboot = ssh("sudo -n systemctl reboot", check=False)
        assert reboot.returncode in (0, 255)
        wait_for(lambda: boot_id() != prior_boot, "actual guest host reboot", seconds=120)
        wait_for(lambda: api("/api/health")["status"] == "ok", "service startup after reboot")
        wait_for(lambda: api(recovery_path)["waiting"], "explicit recovery after provider loss")
        booted = record("host-reboot-waiting")
        assert booted["boot_id"] != prior_boot and booted["executions"] == 1
        view = api(recovery_path)
        assert marker in view["history"] and view["can_replay"]
        action = {"id": str(uuid.uuid4()), "kind": "replay", "revision": view["state"]["revision"], "host_revision": view["host_revision"], "instance": view["instance"], "confirm": True}
        assert api(recovery_path, action)["outcome"] == "consumed"
        wait_for(lambda: executions() == 2, "approved continuation after reboot")
        assert api(recovery_path, action)["outcome"] == "consumed"
        for _ in range(3):
            api("/api/sessions/" + session_id + "/reconnect", {})
        wait_for(lambda: api("/api/sessions/" + session_id)["state"] == "connected", "repeated reconnect")
        recovered = record("approved-once-and-reconnected")
        assert recovered["executions"] == 2
        assert set(original["transcripts"]).issubset(set(recovered["transcripts"]))
        print(json.dumps({"passed": True, "evidence": str(result_file), "includes_update": bool(args.update_binary)}), flush=True)
    finally:
        try:
            api("/api/sessions/" + session_id, method="DELETE")
        finally:
            ssh("tmux kill-session -t " + target, check=False)


if __name__ == "__main__":
    main()
