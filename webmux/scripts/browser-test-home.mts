import fs from 'node:fs';
import path from 'node:path';
import { randomUUID } from 'node:crypto';

export function prepareBrowserTestHome(home: string, retain: boolean): string | undefined {
  if (!fs.existsSync(home)) return;
  if (!retain) { fs.rmSync(home, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }); return; }
  // An unconfirmed Windows launch can outlive its controller. Retain all saved
  // access, keys, logs and the original executable before allocating a new home.
  const retainedRoot = path.join(path.dirname(home), '.test-home-retained');
  fs.mkdirSync(retainedRoot, { recursive: true, mode: 0o700 });
  const destination = path.join(retainedRoot, randomUUID());
  fs.renameSync(home, destination);
  return destination;
}
