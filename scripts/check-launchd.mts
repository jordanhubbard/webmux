#!/usr/bin/env node
// launchctl bootstrap/kickstart can succeed even when the server immediately exits.
import { execFileSync, spawn } from 'node:child_process';
import fs from 'node:fs';
import { setTimeout } from 'node:timers/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

function launch(args: string[], timeout = 2000): string {
  try {
    return execFileSync('launchctl', args, { encoding: 'utf8', timeout, stdio: ['ignore', 'pipe', 'pipe'] });
  } catch (error) {
    const result = error as Error & { stderr?: string };
    throw new Error(`launchctl ${args.join(' ')}: ${result.stderr?.trim() || result.message}`);
  }
}

function state(service: string): string {
  try { return launch(['print', service], 1000); }
  catch (error) {
    if (error instanceof Error && /Could not find service/.test(error.message)) return '';
    throw error;
  }
}

// bootout can return before launchd has finished unregistering the previous job.
// Retry registration, not kickstart -k (which would kill the newly started job).
export async function registerService(domain: string, plist: string,
  bootstrap: () => void = () => { launch(['bootstrap', domain, plist]); },
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

async function checkService(service: string, log: string, plist?: string): Promise<void> {
  const domain = service.slice(0, service.lastIndexOf('/'));
  if (plist) {
    try { launch(['bootout', service], 5000); }
    catch (error) {
      // Ignore only an absent job, not a failed stop of an existing service.
      if (state(service)) throw error;
    }
    const deadline = Date.now() + 10000;
    while (state(service)) {
      if (Date.now() >= deadline) throw new Error(`Timed out waiting for removal of ${service}`);
      await setTimeout(200);
    }
    await registerService(domain, plist);
  }
  const deadline = Date.now() + 10000;
  let previous = '', since = 0;
  while (Date.now() < deadline) {
    const output = state(service);
    // A successful bootstrap is not proof that the replacement remains registered.
    // Re-register absent jobs, but let registered jobs start without killing them.
    if (!output && plist) {
      await registerService(domain, plist, undefined, undefined, Math.max(0, deadline - Date.now()));
    }
    const pid = /^\s*state = running\s*$/m.test(output) ? /^\s*pid = (\d+)\s*$/m.exec(output)?.[1] ?? '' : '';
    if (pid && pid === previous) {
      if (Date.now() - since >= 1000) return;
    } else { previous = pid; since = Date.now(); }
    await setTimeout(200);
  }
  throw new Error(`WebMux did not remain running under ${service}; check ${log}`);
}

async function main(): Promise<void> {
  const [service, log, plist, worker] = process.argv.slice(2);
  if (!service || !log) throw new Error('Usage: check-launchd.mts SERVICE LOGFILE [PLIST]');
  if (plist && worker !== '--worker') {
    // Installing from a WebMux terminal can close that terminal during bootout.
    // Start a new session and disconnect all stdio before touching the service.
    const fd = fs.openSync(log, 'a', 0o600);
    console.log(`Replacing ${service}; installation will continue if this terminal closes. Progress: ${log}`);
    try {
      const child = spawn(process.execPath, [process.argv[1], service, log, plist, '--worker'], {
        detached: true, stdio: ['ignore', fd, fd],
      });
      const status = await new Promise<number>((resolve, reject) => {
        child.once('error', reject);
        child.once('exit', code => resolve(code ?? 1));
      });
      process.exitCode = status;
      if (status !== 0) console.error(`WebMux service installation failed; check ${log}`);
    } finally { fs.closeSync(fd); }
    return;
  }
  try {
    await checkService(service, log, plist);
    if (plist) console.log(`WebMux service installation completed: ${service}`);
  } catch (error) {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  }
}
if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) await main();
