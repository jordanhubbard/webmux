import { expect, type Page } from '@playwright/test';
import { spawnSync, type ChildProcess } from 'node:child_process';

/** Verify keyboard delivery at the private X display, beyond the browser/proxy. */
export async function verifyXKeyboard(
  page: Page,
  displayName: string,
  start: (command: string, args: string[]) => ChildProcess,
  log: (chunk: string) => void,
): Promise<void> {
  let events = '';
  let failure: Error | undefined;
  // The caller owns cleanup for every child started through this callback.
  const observer = start('xev', ['-display', displayName, '-name', 'webmux-keyboard-fixture', '-geometry', '100x100+400+300']);
  observer.on('error', error => { failure = error; });
  if (!observer.stdout) throw new Error('X event observer requires piped stdout');
  observer.stdout.on('data', chunk => {
    events = (events + String(chunk)).slice(-65536);
    log(String(chunk));
  });
  const xdo = (args: string[]) => {
    const result = spawnSync('xdotool', args, {
      encoding: 'utf8', timeout: 5000, env: { ...process.env, DISPLAY: displayName },
    });
    if (result.error) throw result.error;
    return result;
  };
  let window = '';
  await expect.poll(() => {
    if (failure) throw failure;
    const found = xdo(['search', '--name', '^webmux-keyboard-fixture$']);
    window = found.stdout.trim();
    return found.status === 0 && /^\d+$/.test(window);
  }).toBe(true);
  const focused = xdo(['windowfocus', window]);
  expect(focused.status, focused.stderr).toBe(0);
  await page.keyboard.press('a');
  await page.keyboard.press('Enter');
  await expect.poll(() => Array.from(
    events.matchAll(/(KeyPress|KeyRelease) event,[\s\S]*?keysym ([^)\n]+)/g),
    match => `${match[1]} ${match[2]}`,
  )).toEqual(expect.arrayContaining([
    'KeyPress 0x61, a', 'KeyRelease 0x61, a',
    'KeyPress 0xff0d, Return', 'KeyRelease 0xff0d, Return',
  ]));
}
