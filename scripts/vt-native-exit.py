#!/usr/bin/env python3
"""Retain native child status without changing its diagnostic output stream."""
import json
from pathlib import Path
import subprocess
import sys
import time


def run(report: Path, command: list[str]) -> int:
    started = time.monotonic()
    result = {"platform": sys.platform, "started": False, "exit_code": None,
              "windows_exit_hex": None, "spawn_error": None}
    try:
        child = subprocess.Popen(command, stdin=subprocess.DEVNULL)
    except OSError as error:
        result["spawn_error"] = getattr(error, "winerror", None) or error.errno
    else:
        result["started"] = True
        result["exit_code"] = child.wait()
        if sys.platform == "win32":
            result["windows_exit_hex"] = f"0x{result['exit_code'] & 0xffffffff:08x}"
    result["elapsed_seconds"] = round(time.monotonic() - started, 6)
    report.write_text(json.dumps(result, indent=2) + "\n")
    return 0 if result["started"] and result["exit_code"] == 0 else 1


if __name__ == "__main__":
    if len(sys.argv) < 3:
        sys.exit("Usage: vt-native-exit.py REPORT COMMAND [ARG ...]")
    sys.exit(run(Path(sys.argv[1]), sys.argv[2:]))
