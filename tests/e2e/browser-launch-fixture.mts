// Simulate the CLI's Enter-to-launch contract, including captured child output.
import { readFileSync } from 'node:fs';
import { spawn } from 'node:child_process';

const { url, mode } = JSON.parse(readFileSync(process.argv[2], 'utf8')) as { url: string; mode: string };
process.stdout.write('Press Enter to launch the authentication browser.\n');
process.stdin.once('data', () => {
  const command = mode === 'gh-browser' ? process.env.GH_BROWSER : mode === 'browser' ? process.env.BROWSER : mode;
  if (!command) throw new Error('Browser environment was not installed');
  const child = spawn(command, [url], { stdio: ['ignore', 'pipe', 'pipe'] });
  let diagnostics = '';
  child.stdout.on('data', chunk => { diagnostics += String(chunk); });
  child.stderr.on('data', chunk => { diagnostics += String(chunk); });
  child.on('error', error => { console.error(error); process.exit(1); });
  child.on('exit', code => {
    if (code !== 0) { console.error(diagnostics); process.exit(code ?? 1); }
    process.stdout.write('Browser launch accepted.\n');
    process.exit(0);
  });
});
