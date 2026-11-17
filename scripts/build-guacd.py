#!/usr/bin/env python3
"""Build the qualified, RDP-enabled guacd into a new private prefix on Linux."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess

VERSION = "1.6.0"
SHA256 = "8bc45675da96d7b6f39728160181e3d4ff3c08f460f6d26de5805b642bf13f2b"
SIGNER = "F467E54ACC52F1D2778826865B2977AEE5E4518F"
SOURCE = f"https://archive.apache.org/dist/guacamole/{VERSION}/source/guacamole-server-{VERSION}.tar.gz"


def digest(path):
    result = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1 << 20), b""):
            result.update(block)
    return result.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prefix", required=True, type=Path)
    parser.add_argument("--build-root", required=True, type=Path)
    parser.add_argument("--jobs", default=2, type=int)
    args = parser.parse_args()
    if platform.system() != "Linux" or not 1 <= args.jobs <= 32:
        parser.error("requires Linux and 1..32 build jobs")
    prefix, root = args.prefix.resolve(), args.build_root.resolve()
    for location in (prefix, root):
        if location.exists() and (not location.is_dir() or any(location.iterdir())):
            parser.error(f"refusing to overwrite nonempty path: {location}")
    if prefix == root or root in prefix.parents or prefix in root.parents:
        parser.error("prefix and build-root must be separate directories")
    for command in ("curl", "gpg", "tar", "make", "cc", "pkg-config"):
        if not shutil.which(command):
            parser.error(f"missing build prerequisite: {command}")
    root.mkdir(parents=True, exist_ok=True)
    prefix.mkdir(parents=True, exist_ok=True)
    log = (root / "build.log").open("wb")

    def run(command, *, cwd=root, timeout=300, env=None):
        log.write(("\n$ " + repr(command) + "\n").encode()); log.flush()
        subprocess.run(command, cwd=cwd, env=env, stdout=log, stderr=log,
                       check=True, timeout=timeout)

    def download(url, target):
        run(["curl", "--fail", "--location", "--retry", "2", "--connect-timeout", "15",
             "--max-time", "120", "--output", str(target), url], timeout=400)

    print(f"Verifying Apache Guacamole {VERSION}", flush=True)
    archive = root / f"guacamole-server-{VERSION}.tar.gz"
    download(SOURCE, archive)
    if digest(archive) != SHA256:
        raise RuntimeError("guacd source checksum mismatch")
    download(SOURCE + ".asc", root / "source.asc")
    download("https://downloads.apache.org/guacamole/KEYS", root / "KEYS")
    keys = root / "keys"; keys.mkdir(mode=0o700)
    run(["gpg", "--homedir", str(keys), "--batch", "--import", str(root / "KEYS")])
    signature = subprocess.run(["gpg", "--homedir", str(keys), "--batch", "--status-fd", "1",
                                "--verify", str(root / "source.asc"), str(archive)],
                               capture_output=True, text=True, check=True, timeout=30)
    (root / "signature.txt").write_text(signature.stdout + signature.stderr)
    if not any(line.startswith("[GNUPG:] VALIDSIG " + SIGNER + " ") for line in signature.stdout.splitlines()):
        raise RuntimeError("guacd release signer mismatch")
    run(["tar", "-xzf", str(archive), "-C", str(root)])
    source = root / f"guacamole-server-{VERSION}"
    configure = ["./configure", f"--prefix={prefix}", "--with-rdp",
                 f"--with-freerdp-plugin-dir={prefix / 'lib/freerdp2'}",
                 f"--with-systemd-dir={prefix / 'lib/systemd/system'}",
                 "--without-vnc", "--without-ssh", "--without-telnet",
                 "--disable-kubernetes", "--disable-guacenc", "--disable-guaclog"]
    print("Building RDP proxy", flush=True)
    run(configure, cwd=source)
    run(["make", f"-j{args.jobs}"], cwd=source, timeout=600)
    run(["make", "install"], cwd=source)
    env = {**os.environ, "LD_LIBRARY_PATH": str(prefix / "lib")}
    executable = prefix / "sbin/guacd"
    plugin = prefix / "lib/libguac-client-rdp.so"
    version = subprocess.check_output([str(executable), "-v"], env=env, text=True, timeout=10).strip()
    if VERSION not in version or not plugin.is_file():
        raise RuntimeError("qualified guacd or RDP plugin missing")
    libraries = subprocess.check_output(["ldd", str(executable), str(plugin)], env=env, text=True, timeout=10)
    if "not found" in libraries:
        raise RuntimeError("guacd has unresolved native libraries")
    (root / "libraries.txt").write_text(libraries)
    receipt = {"version": VERSION, "source": SOURCE, "source_sha256": SHA256,
               "signer": SIGNER, "architecture": platform.machine(),
               "guacd_sha256": digest(executable), "rdp_plugin_sha256": digest(plugin),
               "configure": configure, "runtime_library_path": str(prefix / "lib")}
    (root / "receipt.json").write_text(json.dumps(receipt, indent=2) + "\n")
    log.close()
    print(f"{version}; receipt: {root / 'receipt.json'}", flush=True)


if __name__ == "__main__":
    main()
