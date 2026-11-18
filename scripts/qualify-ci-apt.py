#!/usr/bin/env python3
"""Qualify CI APT source isolation in a disposable Linux worker, using private state."""
import argparse
import hashlib
import http.server
import importlib.util
import json
import os
from pathlib import Path
import platform
import re
import signal
import subprocess
import threading


def qualify(output: Path) -> dict:
    if platform.system() != "Linux":
        raise RuntimeError("Native APT qualification requires Linux")
    output.mkdir(mode=0o755, exist_ok=False)
    config = output / "etc"
    parts = config / "sources.list.d"
    parts.mkdir(parents=True)
    release = dict(line.split("=", 1) for line in Path("/etc/os-release").read_text().splitlines() if "=" in line)
    suite = release.get("VERSION_CODENAME", "").strip('"')
    architecture = subprocess.check_output(["dpkg", "--print-architecture"], text=True).strip()
    keyring = Path("/usr/share/keyrings/ubuntu-archive-keyring.gpg")
    if release.get("ID", "").strip('"') != "ubuntu" or not re.fullmatch(r"[a-z][a-z0-9-]+", suite) or not keyring.is_file():
        raise RuntimeError("APT controls require an Ubuntu worker with its installed archive keyring")
    if architecture == "amd64":
        archive, security = "https://archive.ubuntu.com/ubuntu/", "https://security.ubuntu.com/ubuntu/"
    elif architecture == "arm64":
        archive = security = "https://ports.ubuntu.com/ubuntu-ports/"
    else:
        raise RuntimeError("APT controls currently qualify native AMD64 and ARM64 workers")
    # Declare controlled sources rather than inheriting unrelated live vendor
    # services into the baseline. Real CI provisions its original Ubuntu entries
    # separately; this fixture exercises both APT formats and the actual trust.
    (config / "sources.list").write_text(f"deb [arch={architecture} signed-by={keyring}] {archive} {suite} main\n")
    (parts / "ubuntu.sources").write_text(f"Types: deb\nURIs: {security}\nSuites: {suite}-security\nComponents: main\nArchitectures: {architecture}\nSigned-By: {keyring}\n")
    helper_path = Path(__file__).with_name("configure-ci-apt.py")
    spec = importlib.util.spec_from_file_location("ci_apt", helper_path)
    helper = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(helper)
    bootstrap = output / "bootstrap.conf"
    bootstrap.write_text(f'''Dir::Etc "{config}";
Dir::Etc::main "/dev/null";
Dir::Etc::trusted "/etc/apt/trusted.gpg";
Dir::Etc::trustedparts "/etc/apt/trusted.gpg.d";
Dir::State "{output / 'state'}";
Dir::State::status "/var/lib/dpkg/status";
Dir::Cache "{output / 'cache'}";
Dir::Log "{output / 'log'}";
''')
    for directory in [output / "state/lists/partial", output / "cache/archives/partial", output / "log"]:
        directory.mkdir(parents=True)

    requests = []
    class UnavailableFeed(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            requests.append(self.path)
            self.send_error(429, "Controlled unrelated feed outage")

        def log_message(self, *_):
            pass

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), UnavailableFeed)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    vendor_uri = f"http://127.0.0.1:{server.server_port}/vendor"
    vendor = parts / "unrelated-vendor.list"
    vendor.write_text(f"deb {vendor_uri} fixture main\n")
    vendor_digest = hashlib.sha256(vendor.read_bytes()).hexdigest()
    # Include a legal Deb822 continuation after a comment: it must not escape
    # filtering and become an unintended alternate mirror in the selected copy.
    mixed = False
    for source in sorted(parts.glob("*.sources")):
        text, count = re.subn(r"(?mi)^(URIs:.*)$", lambda m: m[0] + "\n# controlled alternate mirror\n " + vendor_uri, source.read_text(), count=1)
        if count:
            source.write_text(text)
            mixed = True
            break

    def update(label: str, options=()) -> tuple[int, str]:
        log = output / (label + ".log")
        with log.open("w") as stream:
            process = subprocess.Popen(["apt-get", *options, "update"], env={**os.environ, "APT_CONFIG": str(bootstrap)},
                                       stdout=stream, stderr=subprocess.STDOUT, start_new_session=True)
            try:
                code = process.wait(timeout=240)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait()
                raise RuntimeError("Private APT control exceeded its deadline") from None
        return code, log.read_text()

    result = {"passed": False, "platform": platform.system(), "architecture": platform.machine(),
              "apt": subprocess.check_output(["apt-get", "--version"], text=True).splitlines()[0],
              "suite": suite, "archiveKeyringSHA256": hashlib.sha256(keyring.read_bytes()).hexdigest(),
              "helperSHA256": hashlib.sha256(helper_path.read_bytes()).hexdigest(),
              "qualifierSHA256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              "mixedDeb822URIControl": mixed,
              "scope": "Declared signed Ubuntu release/security sources in both APT formats, private configuration/lists/cache and a controlled HTTP 429 feed. System sources, installed trust keyrings and installed packages are unchanged. Full CI separately provisions its original configured Ubuntu entries and mirror aliases."}
    try:
        helper.configure(config)
        baseline_code, _ = update("unscoped-update")
        baseline_requests = len(requests)
        if baseline_code != 100 or baseline_requests < 1:
            raise RuntimeError("Expected the original strict APT failure on the controlled feed")
        helper.configure(config, ubuntu_only=True)
        selected_code, _ = update("selected-update")
        selected_requests = len(requests) - baseline_requests
        if selected_code != 0 or selected_requests != 0:
            raise RuntimeError("Selected signed Ubuntu update failed or contacted the unrelated feed")
        if hashlib.sha256(vendor.read_bytes()).hexdigest() != vendor_digest:
            raise RuntimeError("Original vendor configuration changed")
        snapshot = config / "webmux-ci-sources"
        bad_sources = output / "bad-sources"
        bad_sources.mkdir()
        key = output / "empty-untrusted.gpg"
        key.write_bytes(b"")
        deb822 = (snapshot / "ubuntu.sources").read_text().strip()
        if deb822:
            body = re.split(r"\n[ \t]*\n", deb822)[0]
            body, count = re.subn(r"(?mi)^Signed-By:[^\n]*(?:\n[ \t][^\n]*)*", "Signed-By: " + str(key), body)
            if not count:
                body += "\nSigned-By: " + str(key)
            (bad_sources / "bad.sources").write_text(body + "\n")
        else:
            line = (snapshot / "ubuntu.list").read_text().splitlines()[0]
            match = re.fullmatch(r"(\s*deb(?:-src)?\s+)(?:\[([^]]*)\]\s+)?(.*)", line)
            if not match:
                raise RuntimeError("Cannot construct the signed legacy source control")
            options = [item for item in (match[2] or "").split() if not item.startswith("signed-by=")]
            (bad_sources / "bad.list").write_text(match[1] + "[" + " ".join(options + ["signed-by=" + str(key)]) + "] " + match[3] + "\n")
        bad_lists = output / "bad-lists"
        (bad_lists / "partial").mkdir(parents=True)
        rejected_code, rejected_text = update("untrusted-key-update", ["-o", "Dir::Etc::sourceparts=" + str(bad_sources), "-o", "Dir::State::lists=" + str(bad_lists)])
        if rejected_code != 100 or "NO_PUBKEY" not in rejected_text:
            raise RuntimeError("Expected native signature refusal with the empty untrusted keyring")
        result.update(passed=True, unscopedExit=baseline_code, unscopedVendorRequests=baseline_requests,
                      selectedExit=selected_code, selectedVendorRequests=selected_requests,
                      originalVendorConfigurationRetained=True, untrustedKeyExit=rejected_code, nativeNoPublicKeyRefusal=True)
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)
        (output / "report.json").write_text(json.dumps(result, indent=2) + "\n")
    return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    print(json.dumps(qualify(args.output.resolve())))
