#!/usr/bin/env python3
"""Select signed official Ubuntu sources for ephemeral CI provisioning."""
from pathlib import Path
import os
import re
import sys
from urllib.parse import urlsplit


def ubuntu_uri(value):
    mirrors = {
        "mirror+file:/etc/apt/apt-mirrors.txt": "https://archive.ubuntu.com/ubuntu/",
        "mirror+file:/etc/apt/apt-mirrors-security.txt": "https://security.ubuntu.com/ubuntu/",
    }
    if value in mirrors:
        return mirrors[value]
    u = urlsplit(value)
    hosts = {"archive.ubuntu.com": "/ubuntu", "azure.archive.ubuntu.com": "/ubuntu",
             "security.ubuntu.com": "/ubuntu", "ports.ubuntu.com": "/ubuntu-ports"}
    if (u.scheme not in {"http", "https"} or u.netloc not in hosts
            or u.path.rstrip("/") != hosts[u.netloc] or u.query or u.fragment):
        return None
    host = "archive.ubuntu.com" if u.netloc == "azure.archive.ubuntu.com" else u.netloc
    return f"https://{host}{hosts[u.netloc]}/"


def legacy_sources(text):
    selected = []
    for line in text.splitlines():
        match = re.fullmatch(r"\s*(deb(?:-src)?)\s+(\[[^\]\r\n]*\]\s+)?(\S+)\s+(.+)", line)
        if match is None:
            continue
        kind, options, uri, suite = match.groups()
        chosen = ubuntu_uri(uri)
        if chosen:
            if re.search(r"(?:trusted|allow-insecure|allow-weak|allow-downgrade-to-insecure)\s*=\s*(?:yes|true|1)\b", options or "", re.I):
                raise ValueError("Ubuntu source disables signature verification")
            selected.append(f"{kind} {options or ''}{chosen} {suite}")
    return "\n".join(selected) + ("\n" if selected else "")


def deb822_sources(text):
    selected = []
    for paragraph in re.split(r"\n\s*\n", text):
        fields, key = {}, None
        for line in paragraph.splitlines():
            if not line.strip() or line.lstrip().startswith("#"):
                continue
            if line.startswith((" ", "\t")) and key:
                fields[key][1].append(line)
            else:
                match = re.fullmatch(r"([A-Za-z][A-Za-z0-9-]*):\s*(.*)", line)
                if not match:
                    raise ValueError("Malformed deb822 source")
                key = match[1].lower()
                if key in fields:
                    raise ValueError("Duplicate deb822 field")
                fields[key] = (match[1], [match[2]])
        values = {k: " ".join(lines).strip() for k, (_, lines) in fields.items()}
        if not fields or values.get("enabled", "yes").lower() in {"no", "false", "0"}:
            continue
        uris = [chosen for uri in values.get("uris", "").split() if (chosen := ubuntu_uri(uri))]
        if not uris:
            continue
        if not values.get("types") or not set(values["types"].split()) <= {"deb", "deb-src"} or not values.get("suites"):
            raise ValueError("Incomplete Ubuntu deb822 source")
        if any(values.get(k, "no").lower() in {"yes", "true", "1"} for k in ("trusted", "allow-insecure", "allow-weak", "allow-downgrade-to-insecure")):
            raise ValueError("Ubuntu source disables signature verification")
        fields["uris"] = ("URIs", [" ".join(uris)])
        selected.append("\n".join(name + ": " + lines[0] + "".join("\n" + line for line in lines[1:]) for name, lines in fields.values()))
    return "\n\n".join(selected) + ("\n" if selected else "")


def network_settings(source_parts: Path) -> str:
    return ('Acquire::Retries "2";\nAcquire::http::Timeout "30";\nAcquire::https::Timeout "30";\nDpkg::Lock::Timeout "120";\nAPT::Update::Error-Mode "any";\n'
            f'Dir::Etc::sourcelist "-";\nDir::Etc::sourceparts "{source_parts}";\n')


def configure(root: Path) -> None:
    root = root.resolve()
    if not root.is_dir():
        raise ValueError(f"APT configuration directory not found: {root}")
    sources = [root / "sources.list"]
    sources += sorted((root / "sources.list.d").glob("*.list"))
    sources += sorted((root / "sources.list.d").glob("*.sources"))
    legacy, modern = [], []
    for source in sources:
        if source.is_file():
            text = source.read_text()
            (modern if source.suffix == ".sources" else legacy).append(
                deb822_sources(text) if source.suffix == ".sources" else legacy_sources(text))
    if not any(legacy + modern):
        raise ValueError("No enabled official Ubuntu source found")
    private = root / "webmux-ci-sources"
    settings = root / "apt.conf.d" / "99-webmux-ci-network"
    for path in (private, private / "ubuntu.list", private / "ubuntu.sources", settings, settings.parent):
        if path.is_symlink():
            raise ValueError("Refusing symlink in private APT configuration")
    private.mkdir(exist_ok=True)
    if any(p.name not in {"ubuntu.list", "ubuntu.sources"} for p in private.iterdir()):
        raise ValueError("Unexpected file in private APT source directory")
    (private / "ubuntu.list").write_text("".join(legacy))
    (private / "ubuntu.sources").write_text("\n".join(modern))
    settings.parent.mkdir(exist_ok=True)
    settings.write_text(network_settings(private))


if __name__ == "__main__":
    root = Path(sys.argv[1]) if len(sys.argv) == 2 else Path("/etc/apt")
    if root.resolve() == Path("/etc/apt").resolve() and os.environ.get("GITHUB_ACTIONS") != "true":
        sys.exit("System APT configuration is restricted to ephemeral GitHub CI")
    configure(root)
    print("Selected signed Ubuntu HTTPS sources and bounded APT networking; original sources retained")
