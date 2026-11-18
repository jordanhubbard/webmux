#!/usr/bin/env python3
"""Use signed official Ubuntu HTTPS archives for ephemeral CI provisioning."""
from pathlib import Path
import argparse
import re


def official_ubuntu_uri(uri: str) -> bool:
    return uri.rstrip("/") in {
        "https://archive.ubuntu.com/ubuntu",
        "https://security.ubuntu.com/ubuntu",
        "https://ports.ubuntu.com/ubuntu-ports",
        "mirror+file:/etc/apt/apt-mirrors.txt",
        "mirror+file:/etc/apt/apt-mirrors-security.txt",
    }


def select_ci_sources(root: Path, sources: list[Path]) -> str:
    """Select configured Ubuntu entries without changing their trust or suites."""
    legacy, deb822 = [], []
    for source in sources:
        if not source.is_file():
            continue
        if source.suffix != ".sources":
            for line in source.read_text().splitlines():
                match = re.match(r"^\s*deb(?:-src)?\s+(?:\[[^]\n]*\]\s+)?(\S+)\s+", line)
                if match and official_ubuntu_uri(match[1]):
                    legacy.append(line)
            continue
        for paragraph in re.split(r"\n[ \t]*\n", source.read_text()):
            lines = paragraph.strip().splitlines()
            fields = [i for i, line in enumerate(lines) if re.match(r"^URIs:", line, re.I)]
            if len(fields) > 1:
                raise ValueError("Duplicate Deb822 URIs fields")
            if not fields:
                continue
            first = fields[0]
            end = first + 1
            while end < len(lines) and (lines[end].startswith((" ", "\t")) or lines[end].startswith("#")):
                end += 1
            continuations = [line for line in lines[first + 1:end] if not line.lstrip().startswith("#")]
            uris = (lines[first].split(":", 1)[1] + " " + " ".join(continuations)).split()
            selected = [uri for uri in uris if official_ubuntu_uri(uri)]
            if selected:
                # Preserve all other fields verbatim, including multiline
                # Signed-By keys, architectures, components and disabled state.
                deb822.append("\n".join(lines[:first] + ["URIs: " + " ".join(selected)] + lines[end:]))
    if not legacy and not deb822:
        raise ValueError("No configured official Ubuntu sources for CI provisioning")
    directory = root / "webmux-ci-sources"
    quoted = str(directory.resolve())
    # apt.conf does not permit escaped quotes or backslashes inside values.
    if any(char in quoted for char in '\\"\r\n'):
        raise ValueError("Unsupported APT configuration path")
    names = {"ubuntu.list", "ubuntu.sources"}
    if directory.is_symlink() or (directory.exists() and (not directory.is_dir() or any(p.name not in names or p.is_symlink() for p in directory.iterdir()))):
        raise ValueError("CI source directory contains unowned configuration")
    directory.mkdir(exist_ok=True)
    # Write both formats on every invocation so a removed source cannot remain
    # active through a previous generated snapshot.
    (directory / "ubuntu.list").write_text("\n".join(legacy) + ("\n" if legacy else ""))
    (directory / "ubuntu.sources").write_text("\n\n".join(deb822) + ("\n" if deb822 else ""))
    return 'Dir::Etc::sourcelist "/dev/null";\nDir::Etc::sourceparts "' + quoted + '";\n'


def configure(root: Path, *, ubuntu_only: bool = False) -> None:
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
        updated = re.sub(r"http://ports\.ubuntu\.com/ubuntu-ports/?", "https://ports.ubuntu.com/ubuntu-ports/", updated)
        if updated != original:
            target.write_text(updated)
    settings = root / "apt.conf.d" / "99-webmux-ci-network"
    settings.parent.mkdir(exist_ok=True)
    scope = select_ci_sources(root, sources) if ubuntu_only else ""
    settings.write_text('Acquire::Retries "2";\nAcquire::http::Timeout "30";\nAcquire::https::Timeout "30";\nDpkg::Lock::Timeout "120";\nAPT::Update::Error-Mode "any";\n' + scope)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", nargs="?", type=Path, default=Path("/etc/apt"))
    parser.add_argument("--ubuntu-only", action="store_true", help="Select only configured official Ubuntu repositories for ephemeral CI provisioning")
    args = parser.parse_args()
    configure(args.root, ubuntu_only=args.ubuntu_only)
    print("Configured official Ubuntu HTTPS mirrors and bounded APT networking")
