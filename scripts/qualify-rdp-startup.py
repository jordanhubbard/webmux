#!/usr/bin/env python3
"""Compare legacy and qualified native guacd under controlled registration order."""
import argparse
import json
import os
from pathlib import Path
import subprocess


def matches_pixel(event, phase, expected):
    rgba = event.get("rgba", [])
    tolerance = 3 if event.get("codec") in ("jpeg", "remotefx") else 0
    return (event.get("phase") == phase and len(rgba) == 4 and rgba[3] == expected[3]
            and all(type(value) is int and 0 <= value <= 255 and abs(value - target) <= tolerance
                    for value, target in zip(rgba[:3], expected[:3])))


def admits_legacy_loss(result, receipt, server_log):
    events = receipt.get("events", [])
    return (result == 1 and receipt.get("stalledPrevious") is False
            and "RDP_SCHEDULE ERROR" not in server_log
            and "initial_size_before_registration=1" in server_log
            and any(matches_pixel(e, "initial", [0, 0, 255, 255]) for e in events)
            and any(matches_pixel(e, "update", [0, 255, 0, 255]) for e in events)
            and any(e.get("kind") == "failure" and e.get("phase") == "reload" for e in events)
            and any(e.get("kind") == "failure-canvases" and e.get("canvases")
                    and any(c.get("width") == 0 and c.get("height") == 0 for c in e["canvases"])
                    and not any(c.get("width") == 1024 and c.get("height") == 768 for c in e["canvases"]) for e in events))


def admits_qualified(result, receipt, server_log):
    events = receipt.get("events", [])
    return (result == 0 and receipt.get("stalledPrevious") is False and "RDP_SCHEDULE ERROR" not in server_log
            and "RDP_SCHEDULE delayed registration;" in server_log
            and all(any(matches_pixel(e, phase, color) for e in events)
                    for phase, color in [("initial", [0, 0, 255, 255]), ("update", [0, 255, 0, 255]),
                                         ("reload", [0, 255, 0, 255]), ("fullscreen", [0, 255, 0, 255])])
            and any(e.get("kind") == "process-start" and e.get("command") == "xev" for e in events)
            and not any(e.get("kind") == "failure" for e in events))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--legacy", type=Path, required=True)
    parser.add_argument("--qualified", type=Path, required=True)
    parser.add_argument("--library-path", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    output = args.output.resolve()
    if output.exists() and any(output.iterdir()):
        parser.error("output must be new or empty; preserve previous evidence")
    output.mkdir(parents=True, exist_ok=True)
    shim = output / "registration.so"
    subprocess.run(["cc", "-Wall", "-Wextra", "-Werror", "-shared", "-fPIC",
                    str(root / "tests/e2e/guacd-registration-fixture.c"), "-ldl", "-pthread", "-o", str(shim)],
                   check=True, timeout=30)
    results = []
    for name, executable in [("legacy", args.legacy), ("qualified", args.qualified)]:
        env = {**os.environ, "WEBMUX_REAL_RDP": "1", "WEBMUX_GUACD_EXECUTABLE": str(executable.resolve()),
               "WEBMUX_GUACD_REGISTRATION_SHIM": str(shim)}
        if name == "qualified":
            env["LD_LIBRARY_PATH"] = str(args.library_path.resolve())
        else:
            env.pop("LD_LIBRARY_PATH", None)
        target = output / name
        command = ["npm", "run", "test:e2e", "--", "rdp-integration.spec.ts", "--grep-invert",
                   "unresponsive previous viewer", "--max-failures=1", "--output=" + str(target)]
        with (output / (name + ".log")).open("wb") as log:
            result = subprocess.run(command, cwd=root / "webmux", env=env, stdout=log, stderr=log, timeout=240)
        receipts = [p for p in target.rglob("rdp-reconnect.json") if "attachments" not in p.parts]
        if len(receipts) != 1:
            raise RuntimeError(f"{name}: missing or ambiguous native RDP receipt")
        receipt = json.loads(receipts[0].read_text())
        server_log = receipts[0].with_name("rdp-server.log").read_text()
        admitted = (admits_legacy_loss if name == "legacy" else admits_qualified)(result.returncode, receipt, server_log)
        results.append({"daemon": name, "exit": result.returncode, "admitted": admitted})
        (output / "receipt.json").write_text(json.dumps({"results": results}, indent=2) + "\n")
        if not admitted:
            raise RuntimeError(f"{name}: unexpected native result; inspect {target}")
        print(f"{name}: expected native outcome verified", flush=True)


if __name__ == "__main__":
    main()
