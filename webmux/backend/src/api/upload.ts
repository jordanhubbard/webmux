import { Router, Request, Response } from 'express';
import { requireAuth } from '../middleware/auth';
import * as fs from 'fs';
import * as path from 'path';
import * as crypto from 'crypto';

const router = Router();
router.use(requireAuth);

const UPLOAD_DIR = path.join(process.env.WEBMUX_HOME || path.join(process.env.HOME || '/tmp', '.config/webmux'), 'uploads');
const MAX_SIZE = 10 * 1024 * 1024; // 10 MB

// Allowed filename characters (prevent path traversal)
const SAFE_NAME_RE = /^[a-zA-Z0-9._-]+$/;

router.post('/', (req: Request, res: Response) => {
  const contentType = req.headers['content-type'] || '';
  if (!contentType.startsWith('application/octet-stream')) {
    res.status(400).json({ error: 'Content-Type must be application/octet-stream' });
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
    if (size > MAX_SIZE) {
      rejected = true;
      res.status(413).json({ error: 'File too large (max 10 MB)' });
      req.resume(); // drain remaining data so client receives the 413 response
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
