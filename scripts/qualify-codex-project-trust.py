#!/usr/bin/env python3
"""Verify native project-trust refusal without authentication or a coding turn."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time

from importlib.util import spec_from_file_location, module_from_spec


def sha(file):
    return hashlib.sha256(file.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--codex", required=True)
    parser.add_argument("--node", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    codex, node = [Path(shutil.which(p) or p).resolve() for p in [args.codex, args.node]]
    assert subprocess.check_output([str(codex), "--version"], text=True, timeout=5).strip() == "codex-cli 0.169.0"
    source = Path(__file__).resolve()
    spec = spec_from_file_location("startup_controls", source.with_name("qualify-codex-observer-startup.py"))
    controls = module_from_spec(spec)
    spec.loader.exec_module(controls)
    module = source.parents[1] / "webmux/server/internal/openrigcoord/codex-owner-plan.mts"
    root = Path(tempfile.mkdtemp(prefix="wm-owner-trust-")).resolve()
    (root / "mcp.py").write_text(controls.MCP)
    report = {"scope": "Native 0.169 untrusted project configuration; disposable homes, no authentication or coding turn",
              "cliSHA256": sha(codex), "moduleSHA256": sha(module), "probeSHA256": sha(source),
              "cases": [], "passed": False}
    try:
        for mode in ["native-explicit-cwd-control", "actual-owner-resolver"]:
            home = root / mode
            home.mkdir(mode=0o700)
            cwd = home / "project"
            cwd.mkdir()
            (cwd / ".codex").mkdir()
            subprocess.run(["git", "init", str(cwd)], check=True, capture_output=True, timeout=5)
            marker = home / "mcp.marker"
            config = home / "config.toml"
            original = 'sandbox_mode="danger-full-access"\napproval_policy="never"\ncli_auth_credentials_store="file"\n'
            config.write_text(original)
            (cwd / ".codex/config.toml").write_text(
                'sandbox_mode="read-only"\n[mcp_servers.project_marker]\ncommand=' + json.dumps(os.sys.executable)
                + '\nargs=' + json.dumps([str(root / "mcp.py"), str(marker)]) + '\n')
            case = {"mode": mode}
            report["cases"].append(case)
            if mode == "native-explicit-cwd-control":
                native = controls.Native(codex, cwd, home)
                try:
                    native.rpc("initialize", {"clientInfo": {"name": "webmux-project-trust-control", "version": "1"}, "capabilities": {"experimentalApi": True}})
                    native.process.stdin.write(b'{"method":"initialized"}\n')
                    native.process.stdin.flush()
                    cfg = native.rpc("config/read", {"cwd": str(cwd), "includeLayers": True})
                    assert any(layer["name"]["type"] == "project" and layer.get("disabledReason") for layer in cfg["layers"])
                    thread = native.rpc("thread/start", {"cwd": str(cwd), "ephemeral": True})
                    until = time.monotonic() + 5
                    while not marker.exists() and time.monotonic() < until:
                        time.sleep(.05)
                    assert thread["sandbox"]["type"] == "readOnly" and config.read_text() != original and marker.exists()
                    case.update({"disabledProjectLayerBefore": True, "homeConfigChanged": True, "projectMcpStarted": True})
                finally:
                    native.close()
                    case["ownedAppServerStopped"] = native.process.poll() is not None
            else:
                env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"), "HOME": str(home), "CODEX_HOME": str(home), "LANG": "en_US.UTF-8"}
                code = 'import {resolveCodexOwnerPlan} from ' + json.dumps(module.as_uri()) + ';try{await resolveCodexOwnerPlan(process.argv[1],{executable:process.argv[2]});process.exitCode=1}catch(e){if(e.message!=="native_project_trust_required")throw e;console.log(e.message)}'
                result = subprocess.run([str(node), "--input-type=module", "-e", code, str(cwd), str(codex)], env=env, text=True, capture_output=True, timeout=30)
                assert result.returncode == 0 and result.stdout.strip() == "native_project_trust_required", "Actual resolver did not refuse untrusted project"
                assert config.read_text() == original and not marker.exists()
                assert not (home / "sessions").exists()
                case.update({"refusal": result.stdout.strip(), "homeConfigUnchanged": True, "projectMcpStarted": False, "noPersistedSession": True})
        assert sha(module) == report["moduleSHA256"]
        report["passed"] = True
    finally:
        args.output.write_text(json.dumps(report, indent=2) + "\n")
        if report["passed"]:
            shutil.rmtree(root)
    print(json.dumps({"passed": report["passed"], "cases": len(report["cases"])}))


if __name__ == "__main__":
    main()
