import { Router, Request, Response } from 'express';
import express from 'express';
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

// Parse raw body with size limit — Express returns 413 automatically if exceeded
router.post('/',
  express.raw({ type: 'application/octet-stream', limit: MAX_SIZE }),
  (req: Request, res: Response) => {
    if (!Buffer.isBuffer(req.body) || req.body.length === 0) {
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
    fs.writeFileSync(filePath, req.body);
    res.status(201).json({ path: filePath, name: safeName, size: req.body.length });
  }
);

// Handle payload-too-large from express.raw()
import type { NextFunction } from 'express';
router.use((err: Error & { status?: number; type?: string }, _req: Request, res: Response, _next: NextFunction) => {
  if (err.type === 'entity.too.large' || err.status === 413) {
    res.status(413).json({ error: 'File too large (max 10 MB)' });
    return;
  }
  res.status(500).json({ error: 'Upload failed' });
});

export default router;
