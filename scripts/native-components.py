#!/usr/bin/env python3
"""Record local native tool identities and an optional live Linux guacd inventory.

This is an observation, not a vulnerability scan or a complete deployment SBOM.
No process arguments, environment, credentials or network destinations are read.
"""
import argparse
from contextlib import contextmanager
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import stat
import subprocess
import time

TOOLS = {"ssh": ["-V"], "mosh": ["--version"], "mosh-client": ["--version"],
         "mosh-server": ["--version"], "tmux": ["-V"], "guacd": ["-v"]}
MAX_MAPS = 2 << 20


@contextmanager
def regular_file(path):
    """Refuse special files without waiting for a FIFO writer or reading devices.

    Validate the opened descriptor, rather than a preceding path stat: the path
    can be replaced between validation and open. NONBLOCK has no effect on
    regular files; BINARY preserves byte hashes on Windows.
    """
    if not stat.S_ISREG(os.stat(path).st_mode):
        raise ValueError(f"Not a regular file: {path}")
    flags = os.O_RDONLY | getattr(os, "O_NONBLOCK", 0) | getattr(os, "O_BINARY", 0)
    with os.fdopen(os.open(path, flags), "rb") as source:
        if not stat.S_ISREG(os.fstat(source.fileno()).st_mode):
            raise ValueError(f"Not a regular file: {path}")
        yield source


def command(argv, timeout=5):
    try:
        result = subprocess.run(argv, capture_output=True, text=True, errors="replace", timeout=timeout)
        output = result.stdout + result.stderr
        return {"state": "ok" if result.returncode == 0 else "error", "exit": result.returncode,
                "output": output[:16384], "truncated": len(output) > 16384}
    except subprocess.TimeoutExpired:
        return {"state": "timeout"}
    except OSError as error:
        return {"state": "unavailable", "error": str(error)}


def file_identity(path, deadline=None):
    """Hash a stable on-disk file; never claim these are mapped process bytes."""
    path = Path(path).resolve(strict=True)
    deadline = deadline or time.monotonic() + 5
    value = hashlib.sha256()
    with regular_file(path) as source:
        before = os.fstat(source.fileno())
        if before.st_size > 512 << 20:
            raise ValueError(f"File exceeds hash bound: {path}")
        for chunk in iter(lambda: source.read(1 << 20), b""):
            if time.monotonic() > deadline:
                raise TimeoutError(f"Hash deadline: {path}")
            value.update(chunk)
        after = os.fstat(source.fileno())
    key = lambda value: (value.st_dev, value.st_ino, value.st_size, value.st_mtime_ns, value.st_ctime_ns)
    if key(before) != key(after) or key(after) != key(path.stat()):
        raise RuntimeError(f"File changed while hashing: {path}")
    return {"path": str(path), "sha256": value.hexdigest(), "bytes": after.st_size,
            "device": after.st_dev, "inode": after.st_ino}


def parse_maps(text):
    """Keep mapped file identities; deleted mappings cannot identify current files."""
    mappings = {}
    for line in text.splitlines():
        parts = line.split(None, 5)
        if len(parts) < 5:
            raise ValueError("Malformed process mapping")
        if len(parts) == 5 or not parts[5].startswith("/"):
            continue
        path = parts[5]
        # proc's newline escape is ambiguous with a literal backslash-octal
        # filename. Do not turn ambiguous text into another file lookup.
        if re.search(r"\\[0-7]{3}", path):
            raise ValueError("Escaped mapping path requires manual inventory")
        deleted = path.endswith(" (deleted)")
        if deleted:
            path = path[:-10]
        device = parts[3].split(":")
        if len(device) != 2:
            raise ValueError("Malformed mapping device")
        identity = (int(device[0], 16), int(device[1], 16), int(parts[4]))
        key = (path, identity, deleted)
        mappings[key] = {"path": path, "mappedDeviceMajor": identity[0],
                         "mappedDeviceMinor": identity[1], "mappedInode": identity[2], "deleted": deleted}
    return sorted(mappings.values(), key=lambda item: (item["path"], item["mappedInode"]))


def process_identity(proc):
    if proc.stat().st_uid != os.getuid():
        raise PermissionError("Select a guacd process owned by the current account")
    stat = (proc / "stat").read_text()
    # comm may contain spaces or ')'; starttime is field 22 after the final ')'.
    fields = stat[stat.rfind(")") + 2:].split()
    if len(fields) < 20:
        raise ValueError("Malformed process identity")
    exe = (proc / "exe").stat()
    return (fields[19], exe.st_dev, exe.st_ino)


def copyright_evidence(package):
    path = Path("/usr/share/doc") / package.split(":")[0] / "copyright"
    try:
        with regular_file(path) as source:
            data = source.read((512 << 10) + 1)
        if len(data) > 512 << 10:
            return {"state": "too_large", "path": str(path)}
        licenses = sorted(set(re.findall(r"^License:[ \t]*([^\r\n]+)", data.decode("utf-8", errors="replace"), re.M)))
        return {"state": "recorded", "path": str(path), "sha256": hashlib.sha256(data).hexdigest(),
                "declaredLicenseFields": [value[:256] for value in licenses[:100]],
                "licenseFieldsTruncated": len(licenses) > 100 or any(len(value) > 256 for value in licenses),
                "scope": "Distributor copyright document; fields are declarations, not a license determination"}
    except (OSError, ValueError) as error:
        return {"state": "unavailable", "error": str(error)}


class DebianPackages:
    """Attribute exact paths using dpkg, including verified usr-merge aliases."""
    def __init__(self):
        self.tool = shutil.which("dpkg-query")
        self.cache = {}
        self.deadline = time.monotonic() + 60

    def lookup(self, path):
        if time.monotonic() > self.deadline:
            return {"state": "unavailable", "reason": "Package inventory deadline reached"}
        if not self.tool:
            return {"state": "unsupported", "reason": "No dpkg database; use the distributor's package inventory"}
        candidates = [str(path)]
        if str(path).startswith(("/usr/lib/", "/usr/bin/", "/usr/sbin/")):
            alias = Path(str(path)[4:])
            if alias.exists() and alias.samefile(path):
                candidates.append(str(alias))
        owners = set()
        for candidate in candidates:
            result = command([self.tool, "--search", candidate])
            if result["state"] == "timeout" or result.get("truncated"):
                return {"state": "unavailable", "reason": "Package path query incomplete"}
            if result["state"] not in ("ok", "error") or result.get("exit") not in (0, 1):
                return {"state": "unavailable", "reason": "Package database query failed"}
            for line in result.get("output", "").splitlines():
                owner, separator, owned_path = line.partition(": ")
                if separator and owned_path == candidate:
                    owners.update(p for p in owner.split(", ") if re.fullmatch(r"[a-z0-9][a-z0-9+.-]*(?::[a-z0-9-]+)?", p))
        if not owners:
            return {"state": "unowned", "reason": "No exact dpkg path owner; retain source-build provenance separately"}
        packages = []
        for owner in sorted(owners):
            if owner not in self.cache:
                result = command([self.tool, "--show", "--showformat=${binary:Package}\t${Version}\t${Architecture}\t${source:Package}\n", owner])
                fields = result.get("output", "").strip().split("\t")
                if result["state"] == "ok" and not result.get("truncated") and len(fields) == 4:
                    self.cache[owner] = {"name": fields[0], "version": fields[1], "architecture": fields[2],
                                         "sourcePackage": fields[3], "copyright": copyright_evidence(owner)}
                else:
                    self.cache[owner] = {"name": owner, "state": "metadata_unavailable"}
            packages.append(self.cache[owner])
        return {"state": "recorded", "packages": packages,
                "scope": "Package database attribution; does not prove unmodified package bytes"}


def observe_guacd(pid, packages, proc_root=Path("/proc")):
    proc = proc_root / str(pid)
    before = process_identity(proc)
    executable = os.readlink(proc / "exe")
    if Path(executable).name != "guacd":
        raise ValueError("Selected PID must be a live guacd executable")
    with (proc / "maps").open("rb") as source:
        data = source.read(MAX_MAPS + 1)
    if len(data) > MAX_MAPS:
        raise ValueError("Process mapping inventory exceeds its bound")
    mappings = parse_maps(data.decode("utf-8", errors="strict"))
    files = []
    deadline = time.monotonic() + 90
    if len(mappings) > 512:
        raise ValueError("Process mapped-file inventory exceeds its bound")
    for mapping in mappings:
        item = {**mapping, "state": "deleted" if mapping["deleted"] else "unavailable"}
        if not mapping["deleted"]:
            try:
                # Fonts and other data mappings are not native executable
                # components. Read only the ELF magic before hashing a file.
                with regular_file(mapping["path"]) as source:
                    if source.read(4) != b"\x7fELF":
                        continue
                identity = file_identity(mapping["path"], deadline)
                if (os.major(identity["device"]), os.minor(identity["device"]), identity["inode"]) != (
                        mapping["mappedDeviceMajor"], mapping["mappedDeviceMinor"], mapping["mappedInode"]):
                    item["state"] = "replaced_on_disk"
                else:
                    item.update(state="recorded", file=identity, package=packages.lookup(Path(identity["path"])))
            except (OSError, RuntimeError, ValueError) as error:
                item["error"] = str(error)
        files.append(item)
    if before != process_identity(proc):
        raise RuntimeError("guacd identity changed during observation; no stable inventory")
    plugins = [item["path"] for item in files if Path(item["path"]).name.startswith("libguac-client-")]
    return {"state": "observed", "pid": pid, "startTicks": before[0], "executable": executable,
            "mapsSHA256": hashlib.sha256(data).hexdigest(), "mappedFiles": files, "observedPlugins": plugins,
            "scope": "One maps snapshot of this process only, including observed plugins. Hashes describe matching on-disk files, not memory contents. Unloaded plugins, other workers, remote endpoints and future loads are not inventoried."}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True, help="new private JSON file; never overwritten")
    parser.add_argument("--guacd-pid", type=int, help="your active Linux guacd connection worker, after plugin loading")
    parser.add_argument("--tool", action="append", default=[], metavar="NAME=PATH", help="explicit executable for " + ", ".join(TOOLS))
    args = parser.parse_args()
    if args.output.exists():
        parser.error("output already exists; preserve earlier observations")
    if args.guacd_pid is not None and (platform.system() != "Linux" or args.guacd_pid <= 0):
        parser.error("--guacd-pid requires Linux and a positive process ID")
    overrides = {}
    for value in args.tool:
        name, separator, path = value.partition("=")
        if not separator or name not in TOOLS or not path or name in overrides:
            parser.error("--tool requires one NAME=PATH per known tool")
        overrides[name] = path
    packages = DebianPackages()
    result = {"schema": 1, "observedUTC": datetime.now(timezone.utc).isoformat(), "system": platform.system(),
              "architecture": platform.machine(), "tools": {}, "guacdProcess": {"state": "not_observed"},
              "scope": "Local native inventory only; not a vulnerability verdict, license approval or complete deployment SBOM. Does not change services or install tools."}
    for name, arguments in TOOLS.items():
        selected = overrides.get(name) or shutil.which(name)
        if not selected:
            result["tools"][name] = {"state": "not_found"}
            continue
        try:
            identity = file_identity(selected)
            version = command([identity["path"], *arguments])
            if file_identity(selected) != identity:
                raise RuntimeError("Tool changed during its version probe")
            result["tools"][name] = {"state": "recorded", "file": identity,
                                      "versionProbe": version,
                                      "package": packages.lookup(Path(identity["path"]))}
        except (OSError, RuntimeError, ValueError) as error:
            result["tools"][name] = {"state": "unavailable", "error": str(error)}
    if args.guacd_pid is not None:
        try:
            result["guacdProcess"] = observe_guacd(args.guacd_pid, packages)
        except (OSError, RuntimeError, ValueError) as error:
            parser.exit(1, f"Native process inventory not written: {error}\n")
        selected = result["tools"]["guacd"].get("file")
        process = result["guacdProcess"]
        executable = next((item.get("file") for item in process["mappedFiles"]
                           if item["path"] == process["executable"] and item["state"] == "recorded"), None)
        matches = bool(selected and executable and selected == executable)
        process["versionToolMatchesExecutable"] = matches
        process["versionProbe"] = result["tools"]["guacd"]["versionProbe"] if matches else {
            "state": "not_observed", "reason": "PATH/selected guacd tool does not match this process image"}
    # Exclusive creation also closes the race after the initial usability check.
    fd = os.open(args.output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as output:
        json.dump(result, output, indent=2)
        output.write("\n")
    print(f"Native component observation saved: {args.output}")


if __name__ == "__main__":
    main()
