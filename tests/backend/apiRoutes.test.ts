import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import express from 'express';
import request from 'supertest';

describe('API Routes', () => {
  let tmpDir: string;
  let originalRoot: string | undefined;
  let app: express.Express;

  beforeEach(async () => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-api-'));
    originalRoot = process.env.WEBMUX_ROOT;
    process.env.WEBMUX_ROOT = tmpDir;

    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });

    // Auth mode none so we don't need tokens
    fs.writeFileSync(path.join(configDir, 'auth.yaml'),
      'auth:\n  mode: none\n  bootstrap_required: false\n  username_pattern: ""\n  password_hash: ""\n');
    fs.writeFileSync(path.join(configDir, 'hosts.yaml'),
      'hosts:\n  - id: h1\n    hostname: host1.example.com\n    port: 22\n    tags: [linux]\n    mosh_allowed: false\n');
    fs.writeFileSync(path.join(configDir, 'keys.yaml'), 'keys: []\n');
    fs.writeFileSync(path.join(configDir, 'layout.yaml'),
      'layout:\n  font_size: 14\n  tiles: []\n');
    fs.writeFileSync(path.join(configDir, 'app.yaml'),
      'app:\n  name: webmux\n  listen_host: 0.0.0.0\n  http_port: 8080\n  https_port: 8443\n  secure_mode: false\n  trusted_http_allowed: true\n  default_term:\n    cols: 80\n    rows: 24\n    font_size: 14\n  transport:\n    prefer_mosh: false\n    ssh_fallback: true\n');

    jest.resetModules();

    const { default: hostsRouter } = require('@backend/api/hosts');
    const { default: configRouter } = require('@backend/api/config');
    const { default: sessionsRouter } = require('@backend/api/sessions');
    const { default: keysRouter } = require('@backend/api/keys');
    const { default: authRouter } = require('@backend/api/auth');
    const { sessionBroker } = require('@backend/services/sessionBroker');

    await sessionBroker.initialize();

    app = express();
    app.use(express.json());
    app.use('/api/hosts', hostsRouter);
    app.use('/api/config', configRouter);
    app.use('/api/sessions', sessionsRouter);
    app.use('/api/keys', keysRouter);
    app.use('/api/auth', authRouter);
  });

  afterEach(() => {
    if (originalRoot === undefined) {
      delete process.env.WEBMUX_ROOT;
    } else {
      process.env.WEBMUX_ROOT = originalRoot;
    }
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  // --- Hosts ---

  describe('GET /api/hosts', () => {
    it('returns hosts list', async () => {
      const res = await request(app).get('/api/hosts');
      expect(res.status).toBe(200);
      expect(res.body).toHaveLength(1);
      expect(res.body[0].id).toBe('h1');
    });
  });

  describe('POST /api/hosts', () => {
    it('creates a new host', async () => {
      const res = await request(app)
        .post('/api/hosts')
        .send({ hostname: 'newhost.example.com', port: 2222, tags: ['test'] });
      expect(res.status).toBe(201);
      expect(res.body.hostname).toBe('newhost.example.com');
      expect(res.body.port).toBe(2222);
    });

    it('returns 400 without hostname', async () => {
      const res = await request(app).post('/api/hosts').send({ port: 22 });
      expect(res.status).toBe(400);
    });
  });

  describe('PUT /api/hosts/:id', () => {
    it('updates a host', async () => {
      const res = await request(app)
        .put('/api/hosts/h1')
        .send({ port: 3333 });
      expect(res.status).toBe(200);
      expect(res.body.port).toBe(3333);
      expect(res.body.hostname).toBe('host1.example.com');
    });

    it('returns 404 for unknown host', async () => {
      const res = await request(app)
        .put('/api/hosts/nonexistent')
        .send({ port: 22 });
      expect(res.status).toBe(404);
    });
  });

  describe('DELETE /api/hosts/:id', () => {
    it('deletes a host', async () => {
      const res = await request(app).delete('/api/hosts/h1');
      expect(res.status).toBe(204);
      const list = await request(app).get('/api/hosts');
      expect(list.body).toHaveLength(0);
    });

    it('returns 404 for unknown host', async () => {
      const res = await request(app).delete('/api/hosts/nonexistent');
      expect(res.status).toBe(404);
    });
  });

  // --- Config ---

  describe('GET /api/config', () => {
    it('returns app config', async () => {
      const res = await request(app).get('/api/config');
      expect(res.status).toBe(200);
      expect(res.body.app.name).toBe('webmux');
    });
  });

  describe('PUT /api/config', () => {
    it('updates app config', async () => {
      const res = await request(app)
        .put('/api/config')
        .send({ app: { name: 'updated' } });
      expect(res.status).toBe(200);
      expect(res.body.app.name).toBe('updated');
    });
  });

  describe('GET /api/config/layout', () => {
    it('returns layout', async () => {
      const res = await request(app).get('/api/config/layout');
      expect(res.status).toBe(200);
      expect(res.body.layout).toBeDefined();
    });
  });

  describe('PUT /api/config/layout', () => {
    it('saves layout', async () => {
      const layout = { layout: { font_size: 16, tiles: [] } };
      const res = await request(app).put('/api/config/layout').send(layout);
      expect(res.status).toBe(200);
      expect(res.body.layout.font_size).toBe(16);
    });
  });

  // --- Sessions ---

  describe('POST /api/sessions', () => {
    it('creates a session', async () => {
      const res = await request(app)
        .post('/api/sessions')
        .send({ username: 'user', hostname: 'box.example.com' });
      expect(res.status).toBe(201);
      expect(res.body.hostname).toBe('box.example.com');
    });

    it('returns 400 without username', async () => {
      const res = await request(app)
        .post('/api/sessions')
        .send({ hostname: 'box.example.com' });
      expect(res.status).toBe(400);
    });
  });

  describe('GET /api/sessions', () => {
    it('returns sessions list', async () => {
      await request(app).post('/api/sessions').send({ username: 'u', hostname: 'h' });
      const res = await request(app).get('/api/sessions');
      expect(res.status).toBe(200);
      expect(res.body.length).toBeGreaterThan(0);
    });
  });

  describe('GET /api/sessions/:id', () => {
    it('returns a specific session', async () => {
      const created = await request(app).post('/api/sessions').send({ username: 'u', hostname: 'h' });
      const res = await request(app).get(`/api/sessions/${created.body.id}`);
      expect(res.status).toBe(200);
      expect(res.body.id).toBe(created.body.id);
    });

    it('returns 404 for unknown session', async () => {
      const res = await request(app).get('/api/sessions/nonexistent');
      expect(res.status).toBe(404);
    });
  });

  describe('DELETE /api/sessions/:id', () => {
    it('deletes a session', async () => {
      const created = await request(app).post('/api/sessions').send({ username: 'u', hostname: 'h' });
      const res = await request(app).delete(`/api/sessions/${created.body.id}`);
      expect(res.status).toBe(204);
    });
  });

  describe('POST /api/sessions/:id/split-right', () => {
    it('returns suggested position', async () => {
      const created = await request(app).post('/api/sessions').send({ username: 'u', hostname: 'h', row: 0, col: 0 });
      const res = await request(app).post(`/api/sessions/${created.body.id}/split-right`);
      expect(res.status).toBe(200);
      expect(res.body).toEqual({ row: 0, col: 1 });
    });
  });

  describe('POST /api/sessions/:id/split-below', () => {
    it('returns suggested position', async () => {
      const created = await request(app).post('/api/sessions').send({ username: 'u', hostname: 'h', row: 0, col: 0 });
      const res = await request(app).post(`/api/sessions/${created.body.id}/split-below`);
      expect(res.status).toBe(200);
      expect(res.body).toEqual({ row: 1, col: 0 });
    });
  });

  // --- Keys ---

  describe('GET /api/keys', () => {
    it('returns empty list initially', async () => {
      const res = await request(app).get('/api/keys');
      expect(res.status).toBe(200);
      expect(res.body).toEqual([]);
    });
  });

  describe('POST /api/keys', () => {
    it('creates a key entry', async () => {
      const res = await request(app)
        .post('/api/keys')
        .send({ private_key_path: '/home/user/.ssh/id_rsa', type: 'rsa', description: 'test key' });
      expect(res.status).toBe(201);
      expect(res.body.type).toBe('rsa');
      expect(res.body.description).toBe('test key');
      // Should NOT expose private_key_path
      expect(res.body.private_key_path).toBeUndefined();
    });

    it('returns 400 without private_key_path', async () => {
      const res = await request(app).post('/api/keys').send({ type: 'rsa' });
      expect(res.status).toBe(400);
    });
  });

  describe('DELETE /api/keys/:id', () => {
    it('deletes a key', async () => {
      const created = await request(app)
        .post('/api/keys')
        .send({ private_key_path: '/tmp/key', type: 'ed25519' });
      const res = await request(app).delete(`/api/keys/${created.body.id}`);
      expect(res.status).toBe(204);
    });

    it('returns 404 for unknown key', async () => {
      const res = await request(app).delete('/api/keys/nonexistent');
      expect(res.status).toBe(404);
    });
  });

  // --- Auth ---

  describe('GET /api/auth/status', () => {
    it('returns auth status', async () => {
      const res = await request(app).get('/api/auth/status');
      expect(res.status).toBe(200);
      expect(res.body.mode).toBe('none');
    });
  });

  describe('POST /api/auth/login', () => {
    it('returns token in none mode', async () => {
      const res = await request(app)
        .post('/api/auth/login')
        .send({ username: 'admin', password: 'pass' });
      expect(res.status).toBe(200);
      expect(res.body.token).toBeDefined();
      expect(res.body.mode).toBe('none');
    });

    it('returns 400 without credentials', async () => {
      const res = await request(app).post('/api/auth/login').send({});
      expect(res.status).toBe(400);
    });
  });

  describe('POST /api/auth/bootstrap', () => {
    it('returns 403 when bootstrap not required', async () => {
      const res = await request(app)
        .post('/api/auth/bootstrap')
        .send({ username: 'admin', password: 'pass' });
      expect(res.status).toBe(403);
    });

    it('returns 400 without credentials', async () => {
      const res = await request(app).post('/api/auth/bootstrap').send({});
      expect(res.status).toBe(400);
    });
  });
});
