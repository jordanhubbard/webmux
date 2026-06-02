import { Router, Request, Response } from 'express';
import { requireAuth } from '../middleware/auth';
import { persistence } from '../services/persistenceManager';
import * as fs from 'fs';
import * as path from 'path';
import * as crypto from 'crypto';

const router = Router();
router.use(requireAuth);

const UPLOAD_DIR = path.join(process.env.WEBMUX_HOME || path.join(process.env.HOME || '/tmp', '.config/webmux'), 'uploads');
const MAX_FILE_SIZE = 10 * 1024 * 1024;       // 10 MB per file
const QUOTA_BYTES = 500 * 1024 * 1024;        // 500 MB total across all uploads
const MAX_AGE_MS = 30 * 24 * 60 * 60 * 1000;  // purge unreferenced files older than 30 days

const SAFE_NAME_RE = /^[a-zA-Z0-9._-]+$/;

function listUploadFiles(): string[] {
  if (!fs.existsSync(UPLOAD_DIR)) return [];
  return fs.readdirSync(UPLOAD_DIR).filter(f => {
    try { return fs.statSync(path.join(UPLOAD_DIR, f)).isFile(); }
    catch { return false; }
  });
}

function totalUsage(): number {
  return listUploadFiles().reduce((sum, f) => {
    try { return sum + fs.statSync(path.join(UPLOAD_DIR, f)).size; }
    catch { return sum; }
  }, 0);
}

function referencedPaths(): Set<string> {
  try {
    const cfg = persistence.loadKeys();
    return new Set(cfg.keys.map(k => k.private_key_path));
  } catch {
    return new Set();
  }
}

// Deletes upload files older than 30 days that aren't referenced by keys.yaml.
// Safe to call any time; never deletes a file referenced as a private_key_path.
export function runPurge(): { deleted: number; freedBytes: number } {
  if (!fs.existsSync(UPLOAD_DIR)) return { deleted: 0, freedBytes: 0 };
  const refs = referencedPaths();
  const now = Date.now();
  let deleted = 0;
  let freedBytes = 0;
  for (const f of listUploadFiles()) {
    const fullPath = path.join(UPLOAD_DIR, f);
    if (refs.has(fullPath)) continue;
    try {
      const st = fs.statSync(fullPath);
      if (now - st.mtimeMs > MAX_AGE_MS) {
        fs.unlinkSync(fullPath);
        deleted++;
        freedBytes += st.size;
      }
    } catch { /* skip files we can't stat/unlink */ }
  }
  return { deleted, freedBytes };
}

let purgeTimer: NodeJS.Timeout | null = null;

export function startPurgeTimer(): void {
  // Run once now, then every 24h
  const { deleted, freedBytes } = runPurge();
  if (deleted > 0) {
    console.log(`Upload purge (startup): deleted ${deleted} stale files, freed ${freedBytes} bytes`);
  }
  purgeTimer = setInterval(() => {
    const r = runPurge();
    if (r.deleted > 0) {
      console.log(`Upload purge: deleted ${r.deleted} stale files, freed ${r.freedBytes} bytes`);
    }
  }, 24 * 60 * 60 * 1000);
  if (purgeTimer.unref) purgeTimer.unref();
}

export function stopPurgeTimer(): void {
  if (purgeTimer) {
    clearInterval(purgeTimer);
    purgeTimer = null;
  }
}

router.post('/', (req: Request, res: Response) => {
  const contentType = req.headers['content-type'] || '';
  if (!contentType.startsWith('application/octet-stream')) {
    res.status(400).json({ error: 'Content-Type must be application/octet-stream' });
    return;
  }

  // Opportunistically purge stale files if we're near the quota
  let usageBefore = totalUsage();
  if (usageBefore + MAX_FILE_SIZE > QUOTA_BYTES) {
    runPurge();
    usageBefore = totalUsage();
  }
  if (usageBefore >= QUOTA_BYTES) {
    res.status(507).json({ error: `Upload quota exceeded (${QUOTA_BYTES} bytes total)` });
    return;
  }

  const rawName = req.headers['x-filename'] as string | undefined;
  const ext = rawName ? path.extname(rawName) : '';
  const prefix = crypto.randomBytes(6).toString('hex');
  const safeName = rawName && SAFE_NAME_RE.test(path.basename(rawName))
    ? `${prefix}-${path.basename(rawName)}`
    : `${prefix}${ext || '.bin'}`;

  fs.mkdirSync(UPLOAD_DIR, { recursive: true });

  const filePath = path.join(UPLOAD_DIR, safeName);
  const chunks: Buffer[] = [];
  let size = 0;

  let rejected = false;
  req.on('data', (chunk: Buffer) => {
    if (rejected) return;
    size += chunk.length;
    if (size > MAX_FILE_SIZE) {
      rejected = true;
      res.status(413).json({ error: 'File too large (max 10 MB)' });
      req.resume();
      return;
    }
    if (usageBefore + size > QUOTA_BYTES) {
      rejected = true;
      res.status(507).json({ error: `Upload quota exceeded (${QUOTA_BYTES} bytes total)` });
      req.resume();
      return;
    }
    chunks.push(chunk);
  });

  req.on('end', () => {
    if (rejected || res.writableEnded) return;
    fs.writeFileSync(filePath, Buffer.concat(chunks));
    res.status(201).json({ path: filePath, name: safeName, size });
  });

  req.on('error', () => {
    res.status(500).json({ error: 'Upload failed' });
  });
});

export default router;
