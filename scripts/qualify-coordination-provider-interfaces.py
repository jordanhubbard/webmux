#!/usr/bin/env python3
"""Read-only POSIX MCP interface comparison in empty, private provider profiles."""
import argparse
import asyncio
import hashlib
import http.server
import json
import os
from pathlib import Path
import subprocess
import threading


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


class Qualification:
    def __init__(self, args):
        self.args = args
        self.output = args.output
        self.requests = []
        self.results = {"passed": False}
        qualification = self

        class Refuse(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                self.refuse()

            def do_POST(self):
                self.refuse()

            def do_PUT(self):
                self.refuse()

            def do_DELETE(self):
                self.refuse()

            def refuse(self):
                qualification.requests.append({
                    'method': self.command, 'path': self.path.split('?')[0]})
                self.send_response(503)
                self.end_headers()
                self.wfile.write(b'{"error":"fixture refuses backend operations"}')

            def log_message(self, *_):
                pass

        self.endpoint = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Refuse)
        threading.Thread(target=self.endpoint.serve_forever, daemon=True).start()

    def environment(self, name):
        home = self.output / name
        home.mkdir(mode=0o700)
        # No inherited provider identities, native authentication or API keys.
        env = {key: value for key, value in os.environ.items()
               if key in ('PATH', 'TMPDIR', 'LANG', 'LC_ALL', 'TZ')}
        env.update(
            HOME=str(home), CODEX_HOME=str(home / '.codex'),
            CLAUDE_CONFIG_DIR=str(home / '.claude'),
            OPENRIG_HOME=str(home / '.openrig'),
            OPENRIG_URL=f'http://127.0.0.1:{self.endpoint.server_port}',
            OPENRIG_HOST_SELECTED='local', OPENRIG_BIN_REEXEC='1',
            NODE_DISABLE_COMPILE_CACHE='1')
        for kind in ('CONFIG', 'DATA', 'CACHE', 'STATE', 'RUNTIME'):
            directory = home / ('xdg-' + kind.lower())
            directory.mkdir(mode=0o700)
            variable = 'XDG_RUNTIME_DIR' if kind == 'RUNTIME' else f'XDG_{kind}_HOME'
            env[variable] = str(directory)
        return home, env

    async def probe(self, name, command, read_tool):
        initial_requests = len(self.requests)
        home, env = self.environment(name)
        if name == 'agent-deck-recall':
            config = Path(env['XDG_CONFIG_HOME']) / 'agent-deck'
            config.mkdir(mode=0o700)
            (config / 'config.toml').write_text('[recall]\nenabled = true\n')
        process = await asyncio.create_subprocess_exec(
            *map(str, command), cwd=home, env=env,
            stdin=asyncio.subprocess.PIPE, stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE)
        stderr = asyncio.create_task(process.stderr.read())
        received, sent = [], []

        async def send(method, params, request_id=None):
            row = {'jsonrpc': '2.0', 'method': method, 'params': params}
            if request_id is not None:
                row['id'] = request_id
            sent.append(row)
            process.stdin.write((json.dumps(row) + '\n').encode())
            await process.stdin.drain()
            if request_id is None:
                return None
            while True:
                line = await asyncio.wait_for(process.stdout.readline(), 10)
                if not line:
                    raise RuntimeError(f'{name}: protocol stream ended')
                response = json.loads(line)
                received.append(response)
                if response.get('id') == request_id:
                    if 'error' in response:
                        raise RuntimeError(f'{name}: {response["error"]}')
                    return response['result']

        try:
            initialized = await send('initialize', {
                'protocolVersion': '2025-11-25', 'capabilities': {},
                'clientInfo': {'name': 'webmux-read-only-provider-probe', 'version': '1'}}, 1)
            assert initialized.get('protocolVersion') in (
                '2025-11-25', '2025-06-18', '2024-11-05')
            await send('notifications/initialized', {})
            tools = await send('tools/list', {}, 2)
            assert isinstance(tools.get('tools'), list) and tools['tools']
            assert len(self.requests) == initial_requests, 'metadata contacted a backend'
            assert read_tool['name'] in (
                'rig_status', 'recall_show', 'webmux_coordination_view')
            refusal = await send('tools/call', read_tool, 3)
            assert refusal.get('isError') is True, 'unavailable read became success'
            process.stdin.close()
            await asyncio.wait_for(process.wait(), 10)
            assert process.returncode == 0, (name, process.returncode)
            return {
                'exit_code': process.returncode, 'server_info': initialized['serverInfo'],
                'protocol_version': initialized['protocolVersion'],
                'sent_methods': [row['method'] for row in sent],
                'controlled_read': read_tool['name'], 'controlled_refusal': True,
                'metadata_backend_requests': 0,
                'tool_names': sorted(tool['name'] for tool in tools['tools'])}
        finally:
            if process.returncode is None:
                process.terminate()
                try:
                    await asyncio.wait_for(process.wait(), 5)
                except asyncio.TimeoutError:
                    process.kill()
                    await process.wait()
            try:
                diagnostics = await asyncio.wait_for(stderr, 5)
            except (asyncio.TimeoutError, asyncio.CancelledError):
                diagnostics = b'diagnostic read did not complete'
            (self.output / f'{name}-stderr.log').write_bytes(diagnostics)
            (self.output / f'{name}-protocol.json').write_text(json.dumps({
                'sent_requests': sent, 'received': received,
                'exit_code': process.returncode}, indent=2) + '\n')

    async def run(self):
        args = self.args
        binaries = {'rig_node': args.rig_node, 'rig_entry': args.rig_entry,
                    'agent_deck': args.agent_deck, 'webmux': args.webmux,
                    'qualifier_script': Path(__file__).resolve()}
        before = {name: digest(path) for name, path in binaries.items()}
        try:
            # Bypass an installed shell wrapper that may force its live HOME.
            # --port selects only our refusing endpoint, without daemon startup.
            self.results['rig'] = await self.probe('rig', [
                args.rig_node, args.rig_entry, 'mcp', 'serve', '--port',
                str(self.endpoint.server_port)], {'name': 'rig_status', 'arguments': {}})
            self.results['agent_deck_recall'] = await self.probe(
                'agent-deck-recall', [args.agent_deck, 'recall', 'mcp'], {
                    'name': 'recall_show', 'arguments': {
                        'session': '00000000-0000-4000-8000-000000000000', 'tier': 'card'}})
            self.results['webmux'] = await self.probe(
                'webmux', [args.webmux, 'coordinate', 'mcp'], {
                    'name': 'webmux_coordination_view', 'arguments': {}})
            home, env = self.environment('agent-deck-catalog')
            config = Path(env['XDG_CONFIG_HOME']) / 'agent-deck'
            config.mkdir(mode=0o700)
            (config / 'config.toml').write_text(
                '[recall]\nenabled = false\n\n[mcps.webmux_probe]\n'
                'command = "/fixture/not-executable"\nargs = ["coordinate", "mcp"]\n')
            catalog = subprocess.run(
                [str(args.agent_deck), 'mcp', 'list', '--json'], cwd=home,
                env=env, capture_output=True, text=True, timeout=10)
            (self.output / 'catalog-stdout.json').write_text(catalog.stdout)
            (self.output / 'catalog-stderr.log').write_text(catalog.stderr)
            assert catalog.returncode == 0
            rows = json.loads(catalog.stdout).get('mcps', [])
            assert len(rows) == 1 and rows[0].get('name') == 'webmux_probe'
            assert rows[0].get('command') == '/fixture/not-executable'
            self.results['agent_deck_catalog'] = {
                'exit_code': catalog.returncode, 'fixture_entry_only': True}
            home, env = self.environment('agent-deck-recall-disabled')
            disabled = subprocess.run(
                [str(args.agent_deck), 'recall', 'mcp'], cwd=home, env=env,
                input='', capture_output=True, text=True, timeout=10)
            (self.output / 'recall-disabled-stderr.log').write_text(disabled.stderr)
            assert disabled.returncode == 2 and 'recall is off' in disabled.stderr
            self.results['agent_deck_disabled_recall'] = {
                'exit_code': disabled.returncode, 'expected_refusal': True}
            assert self.requests == [{'method': 'GET', 'path': '/healthz'}]
            assert before == {name: digest(path) for name, path in binaries.items()}
            self.results.update(executable_sha256=before, executables_unchanged=True,
                                backend_requests=self.requests, passed=True)
            return self.results
        finally:
            (self.output / 'receipt.json').write_text(json.dumps({
                **self.results, 'backend_requests': self.requests}, indent=2) + '\n')
            self.endpoint.shutdown()
            self.endpoint.server_close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for flag in ('output', 'rig-node', 'rig-entry', 'agent-deck', 'webmux'):
        parser.add_argument('--' + flag, type=Path, required=True)
    args = parser.parse_args()
    if not __debug__:
        parser.error('Run without Python optimization so qualification assertions stay enabled')
    if os.name != 'posix':
        parser.error('This qualifier requires POSIX HOME/XDG isolation; Windows is unqualified')
    for name in ('rig_node', 'rig_entry', 'agent_deck', 'webmux'):
        path = getattr(args, name).expanduser().resolve(strict=True)
        if not path.is_file():
            parser.error(f'{name} must name a file')
        setattr(args, name, path)
    args.output = args.output.expanduser().absolute()
    os.umask(0o077)
    args.output.mkdir(mode=0o700)  # Preserve all earlier outputs; require a new path.
    result = asyncio.run(Qualification(args).run())
    print(json.dumps({'passed': result['passed'], 'tool_counts': {
        name: len(result[name]['tool_names'])
        for name in ('rig', 'agent_deck_recall', 'webmux')}}))


if __name__ == '__main__':
    main()
