import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import express from 'express';
import request from 'supertest';

describe('File Upload API (/api/upload)', () => {
  let tmpDir: string;
  let originalHome: string | undefined;
  let app: express.Express;

  beforeEach(async () => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-upload-'));
    originalHome = process.env.WEBMUX_HOME;
    process.env.WEBMUX_HOME = tmpDir;

    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });

    // Auth mode none so we don't need tokens
    fs.writeFileSync(path.join(configDir, 'auth.yaml'),
      'auth:\n  mode: none\n  users: []\n');

    app = express();

    const { default: uploadRouter } = require('@backend/api/upload');
    app.use('/api/upload', uploadRouter);
  });

  afterEach(() => {
    if (originalHome !== undefined) {
      process.env.WEBMUX_HOME = originalHome;
    } else {
      delete process.env.WEBMUX_HOME;
    }
    fs.rmSync(tmpDir, { recursive: true, force: true });
    jest.resetModules();
  });

  describe('POST /api/upload', () => {
    it('rejects non-octet-stream content type', async () => {
      const res = await request(app)
        .post('/api/upload')
        .set('Content-Type', 'text/plain')
        .send('hello');
      expect(res.status).toBe(400);
      expect(res.body.error).toMatch(/Content-Type/);
    });

    it('uploads a small file successfully', async () => {
      const content = Buffer.from('hello world');
      const res = await request(app)
        .post('/api/upload')
        .set('Content-Type', 'application/octet-stream')
        .set('x-filename', 'test.txt')
        .send(content);
      expect(res.status).toBe(201);
      expect(res.body).toHaveProperty('path');
      expect(res.body).toHaveProperty('name');
      expect(res.body.size).toBe(content.length);
      // File should end with original name
      expect(res.body.name).toMatch(/test\.txt$/);
      // File should actually exist on disk
      expect(fs.existsSync(res.body.path)).toBe(true);
      expect(fs.readFileSync(res.body.path)).toEqual(content);
    });

    it('uses a random prefix to avoid name collisions', async () => {
      const content = Buffer.from('data');
      const res1 = await request(app)
        .post('/api/upload')
        .set('Content-Type', 'application/octet-stream')
        .set('x-filename', 'same.txt')
        .send(content);
      const res2 = await request(app)
        .post('/api/upload')
        .set('Content-Type', 'application/octet-stream')
        .set('x-filename', 'same.txt')
        .send(content);
      expect(res1.status).toBe(201);
      expect(res2.status).toBe(201);
      expect(res1.body.name).not.toBe(res2.body.name);
    });

    it('sanitizes unsafe filenames', async () => {
      const content = Buffer.from('payload');
      const res = await request(app)
        .post('/api/upload')
        .set('Content-Type', 'application/octet-stream')
        .set('x-filename', '../../../etc/passwd')
        .send(content);
      expect(res.status).toBe(201);
      // The stored name should NOT contain path traversal components
      expect(res.body.name).not.toContain('..');
      expect(res.body.name).not.toContain('/');
    });

    it('handles upload with no x-filename header', async () => {
      const content = Buffer.from('anonymous');
      const res = await request(app)
        .post('/api/upload')
        .set('Content-Type', 'application/octet-stream')
        .send(content);
      expect(res.status).toBe(201);
      expect(res.body).toHaveProperty('name');
      expect(res.body.size).toBe(content.length);
    });

    it('rejects files exceeding 10 MB limit', async () => {
      // Create an 11 MB buffer
      const oversized = Buffer.alloc(11 * 1024 * 1024, 'x');
      const res = await request(app)
        .post('/api/upload')
        .set('Content-Type', 'application/octet-stream')
        .set('x-filename', 'big.bin')
        .send(oversized);
      expect(res.status).toBe(413);
      expect(res.body.error).toMatch(/too large/i);
    });
  });
});
