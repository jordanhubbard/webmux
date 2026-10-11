// A real CLI: starts a callback listener or polls a device endpoint, invokes the
// shell's BROWSER helper, and reports success only when its own flow completes.
import http from 'node:http';
import { readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
const config = JSON.parse(readFileSync(process.argv[2], 'utf8')) as { identity: string; mode: string; verification?: string };
const deadline = setTimeout(() => { console.error('Authentication timed out'); process.exit(1); }, 60_000);
let listener: http.Server | undefined;
function success() {
  clearTimeout(deadline);
  console.log('CLI_AUTHENTICATED_ON_ORIGINATING_HOST');
  listener?.close();
}
let url = config.verification ?? config.identity;
if (config.mode === 'callback') {
  listener = http.createServer((req, res) => {
    const callback = new URL(req.url!, 'http://localhost');
    if (callback.pathname !== '/callback' || callback.searchParams.get('state') !== 'fixture-state' || callback.searchParams.get('code') !== 'fixture-code') {
      res.writeHead(400); res.end(); return;
    }
    res.end('callback accepted'); success();
  });
  await new Promise<void>(resolve => listener!.listen(0, '127.0.0.1', resolve));
  const port = (listener.address() as { port: number }).port;
  url += '?' + new URLSearchParams({ redirect_uri: `http://127.0.0.1:${port}/callback`, state: 'fixture-state' });
} else { console.log('Device code: FIXTURE-1234'); }
const result = spawnSync(process.env.BROWSER!, [url], { stdio: ['ignore', 'pipe', 'pipe'] });
if (result.status !== 0) throw new Error('Browser helper failed');
if (config.mode === 'device') {
  for (;;) {
    const result = await fetch(config.identity + '/token').then(res => res.json()) as { approved: boolean };
    if (result.approved) { success(); break; }
    await new Promise(resolve => setTimeout(resolve, 200));
  }
}
