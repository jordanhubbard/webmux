#!/usr/bin/env node
// launchctl bootstrap/kickstart can succeed even when the server immediately exits.
import { execFileSync } from 'node:child_process';
import { setTimeout } from 'node:timers/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

// bootout can return before launchd has finished unregistering the previous job.
// Retry registration, not kickstart -k (which would kill the newly started job).
export async function registerService(domain: string, plist: string,
  bootstrap: () => void = () => { execFileSync('launchctl', ['bootstrap', domain, plist], { timeout: 2000, stdio: ['ignore', 'pipe', 'pipe'] }); },
  pause = () => setTimeout(200), timeout = 10000): Promise<void> {
  const deadline = Date.now() + timeout;
  for (;;) {
    try { bootstrap(); return; }
    catch (error) {
      if (Date.now() >= deadline) throw error;
      await pause();
    }
  }
}

async function main(): Promise<void> {
  const [service, log, plist] = process.argv.slice(2);
  if (!service || !log) throw new Error('Usage: check-launchd.mts SERVICE LOGFILE');
  if (plist) {
    const domain = service.slice(0, service.lastIndexOf('/'));
    try { execFileSync('launchctl', ['bootout', service], { timeout: 5000, stdio: ['ignore', 'pipe', 'pipe'] }); }
    catch { /* A fresh installation has no previous job. Bootstrap must still succeed. */ }
    try { await registerService(domain, plist); }
    catch (error) { console.error(`Unable to register WebMux under ${service}; check ${log}`); throw error; }
  }
  const deadline = Date.now() + 10000;
  let previous = '', since = 0;
  while (Date.now() < deadline) {
    let output = '';
    try { output = execFileSync('launchctl', ['print', service], { encoding: 'utf8', timeout: 1000, stdio: ['ignore', 'pipe', 'pipe'] }); }
    catch { /* The job may still be starting. */ }
    const pid = /^\s*state = running\s*$/m.test(output) ? /^\s*pid = (\d+)\s*$/m.exec(output)?.[1] ?? '' : '';
    if (pid && pid === previous) {
      if (Date.now() - since >= 1000) return;
    } else { previous = pid; since = Date.now(); }
    await setTimeout(200);
  }
  console.error(`WebMux did not remain running under ${service}; check ${log}`);
  process.exitCode = 1;
}
if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) await main();
