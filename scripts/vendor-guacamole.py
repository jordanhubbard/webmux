#!/usr/bin/env python3
"""Repackage the pinned Apache browser sources as a deterministic local npm input."""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile

VERSION = "1.6.0"
SOURCE_SHA256 = "81f9fd5a7b4377fb0ee295d0d4fec92e9667f2aafaa3d0ed8937f535deabdee4"
SIGNER = "F467E54ACC52F1D2778826865B2977AEE5E4518F"
SOURCE_URL = f"https://archive.apache.org/dist/guacamole/{VERSION}/source/guacamole-client-{VERSION}.tar.gz"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive", type=Path, required=True)
    parser.add_argument("--signature", type=Path, required=True)
    parser.add_argument("--keys", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--check", action="store_true", help="compare without writing")
    args = parser.parse_args()
    raw = args.archive.read_bytes()
    if hashlib.sha256(raw).hexdigest() != SOURCE_SHA256:
        raise SystemExit("Apache source archive checksum mismatch")
    with tempfile.TemporaryDirectory(prefix="webmux-apache-keys-") as home:
        subprocess.run(["gpg", "--homedir", home, "--batch", "--import", str(args.keys.resolve())],
                       capture_output=True, check=True, timeout=30)
        result = subprocess.run(["gpg", "--homedir", home, "--batch", "--status-fd", "1", "--verify",
                                 str(args.signature.resolve()), str(args.archive.resolve())],
                                capture_output=True, text=True, check=True, timeout=30)
        if not any(line.startswith("[GNUPG:] VALIDSIG " + SIGNER + " ") for line in result.stdout.splitlines()):
            raise SystemExit("Apache release signer mismatch")
    prefix = f"guacamole-client-{VERSION}/"
    modules = prefix + "guacamole-common-js/src/main/webapp/"
    files = {}
    with tarfile.open(fileobj=io.BytesIO(raw), mode="r:gz") as source:
        def read(name):
            entry = source.getmember(name)
            if not entry.isfile() or entry.size > 2 << 20:
                raise ValueError("Unexpected Apache source member")
            return source.extractfile(entry).read()
        names = sorted(member.name for member in source.getmembers()
                       if member.name.startswith(modules + "modules/") and member.name.endswith(".js"))
        if not names:
            raise SystemExit("Missing Apache JavaScript modules")
        # Apache's build combines common/license.js and modules/**/*.js. Keep
        # source bytes intact, sort module names, and append only the ESM export.
        names.insert(0, modules + "common/license.js")
        content = {name: read(name) for name in names}
        files["index.js"] = b"\n".join(content.values()) + b"\nexport default Guacamole;\n"
        for name in ("LICENSE", "NOTICE"):
            files[name] = read(prefix + name)
    provenance = {
        "upstream": "Apache Guacamole", "version": VERSION,
        "source": SOURCE_URL, "sourceSHA256": SOURCE_SHA256, "releaseSigner": SIGNER,
        "transformation": "common/license.js, sorted modules/*.js, ESM default export; no runtime source edits",
        "sourceFiles": {name[len(prefix):]: hashlib.sha256(data).hexdigest() for name, data in content.items()},
        "moduleSHA256": hashlib.sha256(files["index.js"]).hexdigest(),
    }
    files["provenance.json"] = (json.dumps(provenance, indent=2) + "\n").encode()
    files["package.json"] = (json.dumps({
        "name": "guacamole-common-js", "version": VERSION + "-webmux.1", "private": True,
        "description": "WebMux ESM packaging of verified Apache Guacamole sources",
        "license": "Apache-2.0", "type": "module", "main": "index.js", "exports": "./index.js",
        "homepage": "https://guacamole.apache.org/",
    }, indent=2) + "\n").encode()
    output = io.BytesIO()
    with gzip.GzipFile(fileobj=output, mode="wb", filename="", mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode="w", format=tarfile.USTAR_FORMAT) as package:
            for name, data in sorted(files.items()):
                info = tarfile.TarInfo("package/" + name)
                info.size, info.mode = len(data), 0o644
                package.addfile(info, io.BytesIO(data))
    artifact = output.getvalue()
    sidecar = dict(provenance, packageSHA256=hashlib.sha256(artifact).hexdigest())
    outputs = {args.output: artifact,
               args.output.parent / "provenance.json": (json.dumps(sidecar, indent=2) + "\n").encode(),
               **{args.output.parent / name: files[name] for name in ("LICENSE", "NOTICE")}}
    for target, data in outputs.items():
        if args.check:
            if target.read_bytes() != data:
                raise SystemExit(f"Vendored file differs from verified Apache sources: {target}")
        else:
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(data)
    print("Verified Apache source/signature; package SHA256 " + sidecar["packageSHA256"])



if __name__ == "__main__":
    main()
