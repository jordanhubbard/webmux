#!/usr/bin/env python3
"""Qualify native exact-ID conversational resume using synthetic content only.

Uses installed CLI authentication, creates two model turns per selected CLI, and
retains its private native conversation and project for later recovery checks.
This does not qualify WebMux transports, interactive CLI switches or host reboot.
"""

import argparse
import concurrent.futures
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import uuid


NAMES = {"codex": "codex", "claude": "claude", "cursor": "agent", "opencode": "opencode"}


def native_binary(name):
    search = os.environ.get("WEBMUX_BROWSER_ORIGINAL_PATH", os.environ.get("PATH", ""))
    for directory in search.split(os.pathsep):
        candidate = shutil.which(name, path=directory)
        if not candidate:
            continue
        # Do not route qualification through the user's current terminal shim.
        opener = Path(directory) / "webmux-open"
        if opener.exists() and opener.resolve() == Path(candidate).resolve():
            continue
        return candidate
    raise RuntimeError("native CLI is not installed on its original PATH")


def private_write(path, content):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as stream:
        stream.write(content)


def records_from(output):
    try:
        value = json.loads(output)
        return value if isinstance(value, list) else [value]
    except ValueError:
        records = []
        for line in output.splitlines():
            try:
                value = json.loads(line)
                if isinstance(value, dict):
                    records.append(value)
            except ValueError:
                pass
        return records


def run_stage(binary, args, project, evidence, stage, timeout):
    environment = dict(os.environ, PWD=str(project))
    process = subprocess.Popen(
        [binary, *args], cwd=project, stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
        start_new_session=True, env=environment,
    )
    timed_out = False
    try:
        output, errors = process.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        timed_out = True
        # Terminate only the private process group created by this invocation.
        try:
            os.killpg(process.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            output, errors = process.communicate(timeout=5)
        except subprocess.TimeoutExpired:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            output, errors = process.communicate()
    private_write(evidence / (stage + ".jsonl"), output)
    private_write(evidence / (stage + ".stderr"), errors)
    if timed_out or process.returncode:
        raise RuntimeError(stage + (" timed out" if timed_out else " exited " + str(process.returncode)))
    records = records_from(output)
    if not records:
        raise RuntimeError(stage + " returned no structured native evidence")
    return records


def qualify(cli, root, timeout):
    evidence = root / (cli + "-" + str(uuid.uuid4()))
    evidence.mkdir(mode=0o700)
    project = evidence / "project"
    project.mkdir(mode=0o700)
    marker = "WM_RECOVERY_" + uuid.uuid4().hex
    result = {
        "cli": cli, "native_session_id": None, "conversational_resume": False,
        "marker": marker, "project": str(project), "evidence": str(evidence),
        "scope": "native headless exact-ID resume; no WebMux transport or reboot claim",
    }
    try:
        binary = native_binary(NAMES[cli])
        version = subprocess.run([binary, "--version"], stdin=subprocess.DEVNULL,
                                 capture_output=True, text=True, timeout=15, check=True)
        result["version"] = version.stdout.strip()
        first = "Remember this exact marker for a later turn in this conversation: " + marker + ". Reply only ready. Do not use tools."
        second = "What exact marker did I ask you to remember earlier in this conversation? Reply with only that marker. Do not use tools."
        session_id = str(uuid.uuid4())
        if cli == "codex":
            base = ["exec", "--ignore-user-config", "--skip-git-repo-check", "--sandbox", "read-only", "--json"]
            create = base + [first]
        elif cli == "claude":
            base = ["--safe-mode", "--tools", "", "--output-format", "json", "-p"]
            create = base + ["--session-id", session_id, first]
        elif cli == "cursor":
            base = ["--mode", "ask", "--sandbox", "enabled", "--print", "--output-format", "json"]
            create = base + [first]
        else:
            base = ["run", "--pure", "--format", "json"]
            create = base + [first]
        records = run_stage(binary, create, project, evidence, "create", timeout)
        for record in records:
            candidate = record.get("session_id") or record.get("sessionID") or record.get("part", {}).get("sessionID")
            if record.get("type") == "thread.started":
                candidate = record.get("thread_id")
            if candidate:
                result["native_session_id"] = candidate
        if cli == "claude":
            result["native_session_id"] = result["native_session_id"] or session_id
        session_id = result["native_session_id"]
        if not session_id:
            raise RuntimeError("missing native conversation ID")
        selector = "resume" if cli == "codex" else "--session" if cli == "opencode" else "--resume"
        records = run_stage(binary, base + [selector, session_id, second], project, evidence, "resume", timeout)
        answers = []
        for record in records:
            answers.extend([record.get("result", ""), record.get("part", {}).get("text", "")])
            if record.get("type") == "item.completed" and record.get("item", {}).get("type") == "agent_message":
                answers.append(record["item"].get("text", ""))
        result["conversational_resume"] = any(isinstance(answer, str) and answer.strip() == marker for answer in answers)
        if not result["conversational_resume"]:
            raise RuntimeError("native resume did not return the remembered marker")
    except (RuntimeError, OSError, subprocess.SubprocessError) as error:
        result["error"] = str(error)
    private_write(evidence / "result.json", json.dumps(result, indent=2) + "\n")
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cli", action="append", choices=NAMES, help="CLI to qualify; repeat to select several (default: all)")
    parser.add_argument("--output", type=Path,
                        default=Path.home() / ".local/share/webmux/recovery-qualification",
                        help="private evidence/projects directory outside existing Git repositories")
    parser.add_argument("--timeout", type=int, default=120, help="seconds per native model turn")
    args = parser.parse_args()
    if os.name != "posix":
        parser.error("this harness requires a POSIX host; native Windows qualification remains separate")
    if args.timeout <= 0:
        parser.error("timeout must be positive")
    root = args.output.resolve()
    root.mkdir(parents=True, exist_ok=True, mode=0o700)
    os.chmod(root, 0o700)
    if shutil.which("git"):
        project = subprocess.run(["git", "-C", str(root), "rev-parse", "--show-toplevel"],
                                 stdin=subprocess.DEVNULL, capture_output=True, timeout=5)
        if project.returncode == 0:
            parser.error("choose --output outside an existing Git repository so native project identity is isolated")
    selected = list(dict.fromkeys(args.cli or NAMES))
    success = True
    results = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=len(selected)) as pool:
        tasks = [pool.submit(qualify, cli, root, args.timeout) for cli in selected]
        for task in concurrent.futures.as_completed(tasks):
            result = task.result()
            results.append(result)
            print(json.dumps(result), flush=True)
            success = success and result["conversational_resume"]
    manifest = root / ("manifest-" + str(uuid.uuid4()) + ".json")
    private_write(manifest, json.dumps(results, indent=2) + "\n")
    print(json.dumps({"manifest": str(manifest)}), flush=True)
    return 0 if success else 1


if __name__ == "__main__":
    raise SystemExit(main())
