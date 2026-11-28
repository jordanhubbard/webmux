#!/usr/bin/env python3
"""Finite three-host credential qualification using dedicated synthetic keys.

Private inventory: {"hub":"ROLE","nodes":{ROLE:{"target":null or "user@host",
"address":"IP","binary":"local cross-built fixture executable"}}}.
Exactly three independent native hosts are required. No MAC runtime is needed.
"""
import argparse
import concurrent.futures
from contextlib import contextmanager
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import ssl
import subprocess
import tarfile
import time
import urllib.error
import urllib.request
import uuid

SSH = ["-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=10"]

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None

def enrolled_hub_pin(directory):
    peers = [peer for peer in directory.get("servers", []) if peer.get("id") == "hub"]
    if len(peers) != 1:
        raise AssertionError("exactly one enrolled hub entry is required for the pin fault")
    pin = peers[0].get("identity", {}).get("tls_key_sha256", "")
    if not isinstance(pin, str) or not re.fullmatch(r"[0-9a-f]{64}", pin):
        raise AssertionError("the enrolled hub must have a complete TLS key pin")
    return pin

def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()

class Node:
    def __init__(self, name, entry, root, config, ca):
        self.name, self.entry, self.root, self.config = name, entry, root, config
        self.ca = ca
        self.url, self.token, self.pid = "", "", None
        self.config_file = str(root / name / "config.json")
        self.executable = str(root / name / "fixture")
        self.runtime = {}

    def remote(self, code, payload):
        argv = ["python3", "-c", code]
        if self.entry.get("target"):
            argv = ["ssh", *SSH, self.entry["target"], shlex.join(argv)]
        result = subprocess.run(argv, input=json.dumps(payload), text=True,
                                capture_output=True, timeout=45, check=False)
        if result.returncode:
            raise RuntimeError(self.name + ": private fixture operation failed")
        return json.loads(result.stdout)

    def copy(self, source, destination):
        source = Path(source)
        if self.entry.get("target"):
            subprocess.run(["scp", "-q", *SSH, str(source),
                            self.entry["target"] + ":" + str(destination)],
                           check=True, stdout=subprocess.DEVNULL,
                           stderr=subprocess.DEVNULL, timeout=45)
        elif source.resolve() != Path(destination).resolve():
            shutil.copyfile(source, destination)

    def stage(self, assets):
        self.remote('''import json,sys
from pathlib import Path
x=json.load(sys.stdin);r=Path(x["root"]);r.mkdir(mode=0o700,exist_ok=True)
d=r/x["name"];d.mkdir(mode=0o700,exist_ok=True)
h=Path(x["home"]);h.mkdir(mode=0o700,exist_ok=True)
(h/".webmux-credential-fixture").write_text("webmux-credential-qualification-v1"+chr(10))
print(json.dumps({"staged":True}))''',
                    {"root": str(self.root), "name": self.name, "home": self.config["home"]})
        for source, destination in [
                (self.entry["binary"], self.executable),
                (self.root / "ca.pem", self.root / "ca.pem"),
                (self.root / self.name / "tls.pem", self.root / self.name / "tls.pem"),
                (self.root / self.name / "tls.key", self.root / self.name / "tls.key"),
                (assets, self.root / "assets.tar.gz")]:
            self.copy(source, destination)
        self.config["defaults"] = str(self.root / "defaults")
        self.config["web"] = str(self.root / "web")
        self.write_config()
        self.remote('''import json,sys,tarfile,hashlib
from pathlib import Path
x=json.load(sys.stdin);r=Path(x["root"]);a=r/"assets.tar.gz"
assert hashlib.sha256(a.read_bytes()).hexdigest()==x["sha"]
with tarfile.open(a) as archive:
 for m in archive:
  parts=Path(m.name).parts
  assert parts and parts[0] in ("defaults","web") and not m.name.startswith("/") and ".." not in parts
  assert m.isdir() or m.isfile()
  p=r/m.name
  if m.isdir():p.mkdir(mode=0o700,parents=True,exist_ok=True)
  else:
   p.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
   with archive.extractfile(m) as source:p.write_bytes(source.read())
   p.chmod(0o600)
Path(x["exe"]).chmod(0o700)
print(json.dumps({"assets_verified":True}))''',
                    {"root": str(self.root), "sha": digest(assets), "exe": self.executable})

    @contextmanager
    def changed_private_config(self, filename, old, new):
        # Fault injection is confined to a marked synthetic fixture, never a live home.
        payload = {"home": self.config["home"], "filename": filename, "old": old, "new": new}
        value = self.remote("""import json,sys,hashlib,os
from pathlib import Path
x=json.load(sys.stdin);h=Path(x["home"])
assert h.name.startswith("wmx-cred-") and (h/".webmux-credential-fixture").is_file()
assert x["filename"] in ("auth.yaml","node-discovery.yaml","servers.yaml")
p=h/"config"/x["filename"];text=p.read_text();assert text.count(x["old"])==1
backup=h.parent/("backup-"+x["filename"])
with backup.open("x") as f:f.write(text)
backup.chmod(0o600)
changed=text.replace(x["old"],x["new"]);temp=p.with_suffix(".qualification")
with temp.open("x") as f:f.write(changed)
temp.chmod(0o600);os.replace(temp,p)
print(json.dumps({"changed_sha":hashlib.sha256(changed.encode()).hexdigest()}))""", payload)
        try:
            yield
        finally:
            self.remote("""import json,sys,hashlib,os
from pathlib import Path
x=json.load(sys.stdin);h=Path(x["home"]);p=h/"config"/x["filename"]
assert hashlib.sha256(p.read_bytes()).hexdigest()==x["sha"]
os.replace(h.parent/("backup-"+x["filename"]),p)
print(json.dumps({"restored":True}))""",
                        {"home": self.config["home"], "filename": filename, "sha": value["changed_sha"]})

    def write_config(self):
        return self.remote('''import json,sys
from pathlib import Path
x=json.load(sys.stdin);p=Path(x["file"]);p.write_text(json.dumps(x["config"]));p.chmod(0o600)
print(json.dumps({"saved":True}))''',
                           {"file": self.config_file, "config": self.config})

    def start(self):
        started = self.remote('''import json,sys,subprocess
from pathlib import Path
x=json.load(sys.stdin);d=Path(x["dir"]);home=d/"private-user-home";home.mkdir(mode=0o700,exist_ok=True)
for name in ("tmp","xdg-config","xdg-data","xdg-cache","tmux"):(d/name).mkdir(mode=0o700,exist_ok=True)
ready=Path(x["ready"])
if ready.exists():ready.unlink()
env={"HOME":str(home),"SHELL":"/bin/bash","PATH":"/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin","TMPDIR":str(d/"tmp"),"XDG_CONFIG_HOME":str(d/"xdg-config"),"XDG_DATA_HOME":str(d/"xdg-data"),"XDG_CACHE_HOME":str(d/"xdg-cache"),"TMUX_TMPDIR":str(d/"tmux"),"WEBMUX_COORDINATION_DISABLED":"1"}
with (d/"fixture.log").open("ab") as log:
 p=subprocess.Popen([x["exe"],"-config",x["config"]],env=env,cwd=d,stdin=subprocess.DEVNULL,stdout=log,stderr=log,start_new_session=True)
(d/"owned-pid").write_text(str(p.pid))
print(json.dumps({"pid":p.pid}))''',
                              {"dir": str(self.root / self.name), "exe": self.executable,
                               "config": self.config_file, "ready": self.config["ready_file"]})
        self.pid = started["pid"]
        deadline = time.monotonic() + 25
        while time.monotonic() < deadline:
            value = self.remote('''import json,sys,platform,socket,hashlib
from pathlib import Path
x=json.load(sys.stdin);p=Path(x["ready"])
if p.exists():
 d=json.loads(p.read_text());d["host_os"]=platform.system().lower();d["host_arch"]=platform.machine()
 d["host_id_sha256"]=hashlib.sha256(socket.gethostname().encode()).hexdigest()
 print(json.dumps(d))
else:print("{}")''', {"ready": self.config["ready_file"]})
            if value.get("ready"):
                self.runtime = value
                self.config["listen"] = value["address"]
                self.write_config()
                self.url = "https://" + value["address"]
                return
            time.sleep(.2)
        raise RuntimeError(self.name + ": fixture readiness failed")

    def stop(self):
        if self.pid is None:
            return
        self.remote('''import json,sys,os,signal,time,subprocess
x=json.load(sys.stdin);pid=x["pid"]
def owned():
 p=subprocess.run(["ps","-ww","-p",str(pid),"-o","uid=,command="],text=True,capture_output=True)
 fields=p.stdout.strip().split(None,1)
 return p.returncode==0 and len(fields)==2 and fields[0]==str(os.getuid()) and x["exe"] in fields[1] and x["config"] in fields[1]
if owned():os.kill(pid,signal.SIGTERM)
end=time.monotonic()+7
while owned() and time.monotonic()<end:time.sleep(.1)
if owned():os.kill(pid,signal.SIGKILL)
end=time.monotonic()+3
while owned() and time.monotonic()<end:time.sleep(.1)
assert not owned(),"owned fixture did not exit"
print(json.dumps({"owned_process_absent":True}))''',
                    {"pid": self.pid, "exe": self.executable, "config": self.config_file})
        self.pid = None

    def api(self, method, route, payload=None, bearer=None, headers=None):
        data = None if payload is None else json.dumps(payload).encode()
        hdr = {"Content-Type": "application/json"}
        if bearer is None:
            bearer = self.token
        if bearer:
            hdr["Authorization"] = "Bearer " + bearer
        if headers:
            hdr.update(headers)
        request = urllib.request.Request(self.url + route, data=data, headers=hdr, method=method)
        opener = urllib.request.build_opener(
            urllib.request.ProxyHandler({}),
            urllib.request.HTTPSHandler(context=ssl.create_default_context(cafile=str(self.ca))),
            NoRedirect())
        try:
            response = opener.open(request, timeout=12)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            body = response.read(32769)
            if len(body) > 32768:
                raise RuntimeError(self.name + ": oversized fixture reply")
            return response.status, json.loads(body) if body else None

    def login(self, username="fixture"):
        code, value = self.api("POST", "/api/auth/login",
                               {"username": username, "password": self.config["account_password"]}, bearer="")
        if code != 200:
            raise RuntimeError(self.name + ": fixture login failed")
        self.token = value["token"]

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--inventory", required=True)
    parser.add_argument("--artifacts", required=True)
    parser.add_argument("--source-head", required=True)
    args = parser.parse_args()
    os.umask(0o077)
    repo = Path(__file__).resolve().parent.parent
    inventory = json.loads(Path(args.inventory).read_text())
    entries, hub_name = inventory["nodes"], inventory["hub"]
    if len(entries) != 3 or hub_name not in entries:
        raise SystemExit("Exactly three native hosts and a designated hub are required")
    for name, entry in entries.items():
        if not re.fullmatch("[a-z][a-z0-9-]{0,31}", name):
            raise SystemExit("Invalid fixture role")
        ipaddress.ip_address(entry["address"])
        if entry.get("target") and not re.fullmatch(r"[A-Za-z0-9_.-]+@[A-Za-z0-9_.:-]+", entry["target"]):
            raise SystemExit("Invalid SSH target")
        if not Path(entry["binary"]).is_file():
            raise SystemExit("Each cross-built fixture binary must exist locally")
    artifacts = Path(args.artifacts).resolve()
    artifacts.mkdir(mode=0o700, parents=True, exist_ok=False)
    root = Path("/tmp") / ("wmx-cred-fleet-" + uuid.uuid4().hex)
    controllers = [entry for entry in entries.values() if not entry.get("target")]
    if len(controllers) != 1:
        raise SystemExit("Exactly one local controller node is required")
    subprocess.run([controllers[0]["binary"], "-initialize", str(root),
                    "-defaults", str(repo / "webmux/config.defaults"), "-web", str(repo / "webmux/web"),
                    "-hosts", ",".join(name + "=" + entry["address"] for name, entry in entries.items())],
                   check=True, capture_output=True, timeout=15)
    manifest = json.loads((root / "manifest.json").read_text())
    assets = artifacts / "public-assets.tar.gz"
    with tarfile.open(assets, "w:gz") as archive:
        archive.add(repo / "webmux/config.defaults", arcname="defaults")
        archive.add(repo / "webmux/web", arcname="web")
    nodes = {name: Node(name, entry, root, manifest["nodes"][name], root / "ca.pem")
             for name, entry in entries.items()}
    hub = nodes[hub_name]
    receivers = [node for name, node in nodes.items() if name != hub_name]
    checks = []
    def check(name, ok, statuses=None):
        checks.append({"name": name, "passed": bool(ok), "statuses": statuses or []})
        if not ok:
            raise AssertionError(name)
    def call(node, method, route, payload, expected, label):
        code, value = node.api(method, route, payload)
        check(label, code == expected, [code])
        return value
    secret_id, links, grants = "", {}, {}
    issued_handles = []
    def wire(node, operation, **extra):
        result = hub.api("POST", "/api/federation/credentials",
                       {"operation": operation, "secret_id": secret_id, "consumer": "ai.openai", **extra},
                       bearer=links[node.name],
                       headers={"X-WebMux-Node-ID": hub.config["node_id"], "X-WebMux-Owner": "fixture"})
        if operation == "issue" and result[0] == 200:
            issued_handles.append(result[1]["handle"])
        return result
    def consume(node, expected, marker=None):
        code, value = node.api("POST", "/api/ai/chat",
                               {"message": "synthetic credential qualification", "history": []})
        check(node.name + ":consumer:" + str(expected),
              code == expected and (marker is None or value.get("reply") == marker), [code])
    failure = None
    try:
        for node in nodes.values():
            node.stage(assets)
            node.start()
            node.login()
        values = [node.runtime for node in nodes.values()]
        check("exact-clean-source-builds",
              all(v.get("build_source_head") == args.source_head and v.get("build_source_modified") == "false" for v in values))
        check("three-independent-native-hosts", len({v["host_id_sha256"] for v in values}) == 3)
        aliases = {"x86_64": "amd64", "aarch64": "arm64"}
        check("native-os-and-architecture",
              all(v["os"] == v["host_os"] and v["arch"] == aliases.get(v["host_arch"], v["host_arch"])
                  for v in values))
        call(hub, "POST", "/api/credentials/setup", {}, 204, "explicit-hub-setup")
        created = call(hub, "POST", "/api/credentials/secrets",
                       {"name": "Native synthetic credential", "value": hub.config["fake_keys"][0],
                        "consumer": "ai.openai", "enabled": True}, 200, "encrypted-secret-create")
        secret_id = created["id"]
        for node in receivers:
            link = "wmx-canary-link-" + uuid.uuid4().hex + uuid.uuid4().hex
            links[node.name] = link
            incoming = {"owner": "fixture", "credential": link, "credential_sharing": False,
                        "source_node_id": node.config["node_id"], "source_owner": "fixture"}
            call(hub, "PUT", "/api/server-grants/" + node.name, incoming, 204, node.name + ":ordinary-link-grant")
            call(node, "PUT", "/api/servers/hub",
                 {"name": "Native fixture hub", "url": hub.url, "credential": link, "allow_http": False},
                 204, node.name + ":peer-configure")
            call(node, "POST", "/api/servers/hub/verify-identity", {}, 204,
                 node.name + ":same-connection-enrollment")
            code, _ = wire(node, "issue")
            check(node.name + ":ordinary-permission-denied", code == 403, [code])
            incoming["credential_sharing"] = True
            call(hub, "PUT", "/api/server-grants/" + node.name, incoming, 204,
                 node.name + ":credential-enrollment-enable")
            code, _ = wire(node, "issue")
            check(node.name + ":named-grant-still-required", code == 403, [code])
            grant = call(hub, "POST", "/api/credentials/grants",
                         {"secret_id": secret_id, "enrollment_id": node.name, "consumer": "ai.openai"},
                         200, node.name + ":named-grant")
            grants[node.name] = grant["id"]
            available = call(node, "GET", "/api/credentials/peers/hub/available?consumer=ai.openai",
                             None, 200, node.name + ":named-metadata")
            check(node.name + ":metadata-is-scoped", len(available) == 1 and available[0]["id"] == secret_id)
            call(node, "PUT", "/api/credentials/selection",
                 {"peer": "hub", "secret_id": secret_id, "consumer": "ai.openai"},
                 204, node.name + ":explicit-selection")
            consume(node, 200, "CREDENTIAL_CANARY_PROVIDER_1")
            for wrong in [{"consumer": "ai.nvidia"}, {"project": "foreign-project"}]:
                code, _ = wire(node, "issue", **wrong)
                check(node.name + ":wrong-scope-denied:" + next(iter(wrong)), code == 403, [code])
        for node in receivers:
            original_token = node.token
            with node.changed_private_config("auth.yaml", "username: fixture", "username: foreign"):
                node.login("foreign")
                code, _ = node.api("GET", "/api/credentials/peers/hub/available?consumer=ai.openai")
                check(node.name + ":foreign-receiving-account-refused", code == 403, [code])
            node.token = original_token
            with node.changed_private_config("node-discovery.yaml", node.config["node_id"], uuid.uuid4().hex):
                consume(node, 403)
            # A changed enrolled destination pin must fail even when the token is valid.
            directory = call(node,"GET","/api/server-directory",None,200,node.name+":current-pin")
            pin = enrolled_hub_pin(directory)
            with node.changed_private_config("servers.yaml",pin,"0"*64):
                consume(node,403)
            consume(node,200,"CREDENTIAL_CANARY_PROVIDER_1")
        first, second = receivers
        code, issued = wire(first, "issue")
        check("handle-issue", code == 200, [code])
        code, _ = wire(second, "redeem", handle=issued["handle"])
        check("wrong-recipient-redeem", code == 403, [code])
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(lambda _: wire(first, "redeem", handle=issued["handle"]), range(2)))
        check("single-use-concurrent-redeem",
              sorted(code for code, _ in results) == [200, 403]
              and sum(value.get("value") == hub.config["fake_keys"][0]
                      for code, value in results if code == 200) == 1,
              [code for code, _ in results])
        code, stale = wire(first, "issue")
        check("rotation-handle-issue", code == 200, [code])
        call(hub, "PUT", "/api/credentials/secrets/" + secret_id,
             {"name": "Native synthetic credential", "value": hub.config["fake_keys"][1],
              "consumer": "ai.openai", "enabled": True}, 200, "rotate-version")
        code, _ = wire(first, "redeem", handle=stale["handle"])
        check("rotated-handle-refused", code == 403, [code])
        for node in receivers:
            consume(node, 200, "CREDENTIAL_CANARY_PROVIDER_2")
        code, before_restart = wire(first, "issue")
        check("restart-handle-issue", code == 200, [code])
        hub.stop()
        hub.start()
        hub.login()
        code, _ = wire(first, "redeem", handle=before_restart["handle"])
        check("restart-invalidates-handle", code == 403, [code])
        for node in receivers:
            consume(node, 200, "CREDENTIAL_CANARY_PROVIDER_2")
        nvidia = call(hub,"POST","/api/credentials/secrets",
                      {"name":"Native NVIDIA synthetic credential","value":hub.config["fake_keys"][0],
                       "consumer":"ai.nvidia","enabled":True},200,"nvidia-secret-create")
        for node in receivers:
            call(hub,"POST","/api/credentials/grants",
                 {"secret_id":nvidia["id"],"enrollment_id":node.name,"consumer":"ai.nvidia"},200,node.name+":nvidia-grant")
            call(node,"PUT","/api/credentials/selection",
                 {"peer":"hub","secret_id":nvidia["id"],"consumer":"ai.nvidia"},204,node.name+":nvidia-select")
            consume(node,200,"CREDENTIAL_CANARY_PROVIDER_1")
            call(node,"PUT","/api/credentials/selection",
                 {"peer":"hub","secret_id":secret_id,"consumer":"ai.openai"},204,node.name+":restore-openai-selection")
        code, expiring = wire(first, "issue")
        check("expiry-handle-issue", code == 200, [code])
        time.sleep(31)
        code, _ = wire(first, "redeem", handle=expiring["handle"])
        check("expired-handle-refused", code == 403, [code])
        call(hub, "PUT", "/api/credentials/secrets/" + secret_id,
             {"name": "Native synthetic credential", "value": "", "consumer": "ai.openai", "enabled": False},
             200, "disable-secret")
        for node in receivers:
            consume(node, 403)
        call(hub, "PUT", "/api/credentials/secrets/" + secret_id,
             {"name": "Native synthetic credential", "value": "", "consumer": "ai.openai", "enabled": True},
             200, "reenable-current-version")
        call(hub, "DELETE", "/api/credentials/grants/" + grants[first.name],
             None, 204, "independent-grant-revocation")
        consume(first, 403)
        consume(second, 200, "CREDENTIAL_CANARY_PROVIDER_2")
        # Editing the receiving enrollment invalidates its named grant and old handles.
        code, prior_enrollment = wire(second,"issue")
        check("enrollment-handle-issue",code==200,[code])
        call(hub,"PUT","/api/server-grants/"+second.name,
             {"owner":"fixture","credential":links[second.name],"credential_sharing":True,
              "source_node_id":second.config["node_id"],"source_owner":"fixture"},204,"reenroll-recipient")
        code,_=wire(second,"redeem",handle=prior_enrollment["handle"])
        check("changed-enrollment-old-handle-refused",code==403,[code])
        consume(second,403)
        # Explicit fresh grant restores access; the old grant cannot self-revive.
        grant=call(hub,"POST","/api/credentials/grants",
                   {"secret_id":secret_id,"enrollment_id":second.name,"consumer":"ai.openai"},200,"explicit-regrant")
        grants[second.name]=grant["id"]
        consume(second,200,"CREDENTIAL_CANARY_PROVIDER_2")
        call(second,"PUT","/api/servers/hub",
             {"name":"Native fixture hub","url":hub.url,"credential":links[second.name],"allow_http":False},204,"replace-local-binding")
        consume(second,403)
        call(second,"PUT","/api/credentials/selection",
             {"peer":"hub","secret_id":secret_id,"consumer":"ai.openai"},204,"explicit-reselect")
        consume(second,200,"CREDENTIAL_CANARY_PROVIDER_2")
        for node in nodes.values():
            metadata = call(node, "GET", "/api/credentials", None, 200, node.name + ":redacted-metadata")
            check(node.name + ":no-key-in-metadata",
                  not any(value in json.dumps(metadata) for value in
                          [*hub.config["fake_keys"], *links.values(), *issued_handles,
                           *(candidate.token for candidate in nodes.values())]))
            if node is hub:
                events=metadata["vault"]["audit"]
                check("audit-retains-grants-and-denials",
                      any(e["operation"]=="grant" and e["allowed"] for e in events)
                      and any(not e["allowed"] for e in events)
                      and any(e["operation"]=="revoke" for e in events))
        custody = hub.remote('''import json,sys,os,stat
from pathlib import Path
x=json.load(sys.stdin);d=Path(x["home"])/"data/credential-hub";k=d/"key";v=d/"vault";data=v.read_bytes()
print(json.dumps({"directory_private":stat.S_IMODE(d.stat().st_mode)==0o700,
"key_private":stat.S_IMODE(k.stat().st_mode)==0o600,"owned":k.stat().st_uid==os.getuid(),
"ciphertext_redacted":not any(s.encode() in data for s in x["values"])}))''',
                             {"home": hub.config["home"], "values": hub.config["fake_keys"]})
        check("native-hub-key-custody", all(custody.values()))
        for node in receivers:
            clean = node.remote('''import json,sys
from pathlib import Path
x=json.load(sys.stdin);h=Path(x["home"]);bad=False
for base in ("config","data","logs"):
 for p in (h/base).rglob("*"):
  if p.is_file() and any(v.encode() in p.read_bytes() for v in x["values"]):bad=True
print(json.dumps({"no_durable_api_key_copy":not bad}))''',
                                {"home": node.config["home"], "values": hub.config["fake_keys"]})
            check(node.name + ":no-durable-key-cache", clean["no_durable_api_key_copy"])
        # Losing or exposing the owner's key file cannot regenerate plaintext custody.
        for operation in ("missing", "unsafe"):
            hub.remote("""import json,sys,os
from pathlib import Path
x=json.load(sys.stdin);h=Path(x["home"]);assert h.name.startswith("wmx-cred-") and (h/".webmux-credential-fixture").is_file()
k=h/"data/credential-hub/key"
if x["operation"]=="missing":k.rename(k.with_name("key.qualification-missing"))
else:k.chmod(0o644)
print(json.dumps({"injected":True}))""", {"home":hub.config["home"],"operation":operation})
            try:
                consume(second,503)
            finally:
                hub.remote("""import json,sys
from pathlib import Path
x=json.load(sys.stdin);k=Path(x["home"])/"data/credential-hub/key"
if x["operation"]=="missing":k.with_name("key.qualification-missing").rename(k)
else:k.chmod(0o600)
print(json.dumps({"restored":True}))""", {"home":hub.config["home"],"operation":operation})
            consume(second,200,"CREDENTIAL_CANARY_PROVIDER_2")
        hub.stop()
        for node in receivers:
            consume(node, 503)
        hub.start();hub.login()
        call(hub,"DELETE","/api/credentials/secrets/"+secret_id,None,204,"delete-secret")
        consume(second,403)
    except Exception as error:
        failure = type(error).__name__ + ": " + str(error) if isinstance(error, AssertionError) else type(error).__name__
    finally:
        for node in reversed(list(nodes.values())):
            try:
                node.stop()
            except Exception:
                failure = failure or "owned fixture cleanup failed"
        result = {
            "schema": "webmux.credential-hub.native-qualification.v1",
            "source_head": args.source_head, "passed": failure is None, "failure": failure, "checks": checks,
            "native_hosts": [
                {"role": node.name,
                 "runtime": {key: value for key, value in node.runtime.items()
                             if key in ("os", "arch", "host_os", "host_arch", "host_id_sha256", "build_source_head", "build_source_modified")},
                 "endpoint_sha256": hashlib.sha256(node.url.encode()).hexdigest(),
                 "executable_sha256": digest(node.entry["binary"])}
                for node in nodes.values()],
            "scope": "Dedicated synthetic API keys and private production-API fixture instances on three distinct native hosts. No real provider login/profile, MAC/OpenShell policy, or live WebMux service changed. Provider acceptance is the explicit synthetic TLS fixture, not native OAuth portability.",
            "cleanup": "Each exact owned fixture process was checked by executable+unique config and stopped; failed fixtures/evidence retained privately."}
        (artifacts / "result.json").write_text(json.dumps(result, indent=2) + "\n")
        print(json.dumps({"passed": result["passed"], "checks": len(checks),
                          "result": str(artifacts / "result.json")}))
    return 0 if failure is None else 1

if __name__ == "__main__":
    raise SystemExit(main())
