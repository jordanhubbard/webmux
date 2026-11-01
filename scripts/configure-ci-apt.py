#!/usr/bin/env python3
"""Use signed official Ubuntu HTTPS archives for ephemeral CI provisioning."""
from pathlib import Path
import re
import sys


def configure(root: Path) -> None:
    if not root.is_dir():
        raise ValueError(f"APT configuration directory not found: {root}")
    for filename, url in {
        "apt-mirrors.txt": "https://archive.ubuntu.com/ubuntu/",
        "apt-mirrors-security.txt": "https://security.ubuntu.com/ubuntu/",
    }.items():
        target = root / filename
        if target.exists():
            target.write_text(url + "\n")
    sources = [root / "sources.list"]
    sources += sorted((root / "sources.list.d").glob("*.list"))
    sources += sorted((root / "sources.list.d").glob("*.sources"))
    for target in sources:
        if not target.is_file():
            continue
        original = target.read_text()
        updated = re.sub(r"https?://(?:azure\.archive|archive)\.ubuntu\.com/ubuntu/?", "https://archive.ubuntu.com/ubuntu/", original)
        updated = re.sub(r"http://security\.ubuntu\.com/ubuntu/?", "https://security.ubuntu.com/ubuntu/", updated)
        if updated != original:
            target.write_text(updated)
    settings = root / "apt.conf.d" / "99-webmux-ci-network"
    settings.parent.mkdir(exist_ok=True)
    settings.write_text('Acquire::Retries "2";\nAcquire::http::Timeout "30";\nAcquire::https::Timeout "30";\nDpkg::Lock::Timeout "120";\nAPT::Update::Error-Mode "any";\n')


if __name__ == "__main__":
    configure(Path(sys.argv[1]) if len(sys.argv) == 2 else Path("/etc/apt"))
    print("Configured official Ubuntu HTTPS mirrors and bounded APT networking")
