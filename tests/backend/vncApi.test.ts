import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import express from 'express';
import request from 'supertest';

describe('VNC API Routes', () => {
  let tmpDir: string;
  let originalHome: string | undefined;
  let app: express.Express;
  let vncBroker: any;

  beforeEach(async () => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-vncapi-'));
    originalHome = process.env.WEBMUX_HOME;
    process.env.WEBMUX_HOME = tmpDir;

    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });

    // Auth mode none — no token needed for most tests
    fs.writeFileSync(
      path.join(configDir, 'auth.yaml'),
      'auth:\n  mode: none\n  users: []\n'
    );
    fs.writeFileSync(
      path.join(configDir, 'hosts.yaml'),
      'hosts:\n' +
      '  - id: h1\n' +
      '    hostname: host1.example.com\n' +
      '    port: 22\n' +
      '    tags: []\n' +
      '    mosh_allowed: false\n' +
      '    vnc_enabled: true\n' +
      '    vnc_port: 5901\n'
    );
    fs.writeFileSync(path.join(configDir, 'keys.yaml'), 'keys: []\n');
    fs.writeFileSync(
      path.join(configDir, 'layout.yaml'),
      'layout:\n  font_size: 14\n  tiles: []\n'
    );
    fs.writeFileSync(
      path.join(configDir, 'app.yaml'),
      'app:\n  name: webmux\n  listen_host: 0.0.0.0\n  http_port: 8080\n  https_port: 8443\n' +
      '  secure_mode: false\n  trusted_http_allowed: true\n' +
      '  default_term:\n    cols: 80\n    rows: 24\n    font_size: 14\n' +
      '  transport:\n    prefer_mosh: false\n    ssh_fallback: true\n'
    );

    jest.resetModules();

    const { default: vncRouter } = require('@backend/api/vnc');
    vncBroker = require('@backend/services/vncBroker').vncBroker;

    await vncBroker.initialize();

    app = express();
    app.use(express.json());
    app.use('/api/vnc', vncRouter);
  });

  afterEach(() => {
    if (originalHome === undefined) {
      delete process.env.WEBMUX_HOME;
    } else {
      process.env.WEBMUX_HOME = originalHome;
    }
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  // ---------------------------------------------------------------------------
  // GET /api/vnc/sessions
  // ---------------------------------------------------------------------------

  describe('GET /api/vnc/sessions', () => {
    it('returns 200 with empty sessions array initially', async () => {
      const res = await request(app).get('/api/vnc/sessions');
      expect(res.status).toBe(200);
      expect(res.body).toEqual([]);
    });

    it('returns only sessions belonging to the authenticated owner', async () => {
      // In auth-mode none, owner defaults to 'anonymous'
      await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc1.example.com' });
      await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc2.example.com' });

      const res = await request(app).get('/api/vnc/sessions');
      expect(res.status).toBe(200);
      expect(res.body).toHaveLength(2);
      expect(res.body.every((s: any) => s.owner === 'anonymous')).toBe(true);
    });
  });

  // ---------------------------------------------------------------------------
  // POST /api/vnc/sessions
  // ---------------------------------------------------------------------------

  describe('POST /api/vnc/sessions', () => {
    it('returns 201 and the created session with valid hostname', async () => {
      const res = await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc.example.com', vnc_port: 5900 });
      expect(res.status).toBe(201);
      expect(res.body.hostname).toBe('vnc.example.com');
      expect(res.body.vnc_port).toBe(5900);
      expect(res.body.id).toBeDefined();
      expect(res.body.kind).toBe('vnc');
    });

    it('returns 201 and resolves hostname from host_id', async () => {
      const res = await request(app)
        .post('/api/vnc/sessions')
        .send({ host_id: 'h1' });
      expect(res.status).toBe(201);
      expect(res.body.hostname).toBe('host1.example.com');
      expect(res.body.vnc_port).toBe(5901);
    });

    it('returns 400 when neither hostname nor host_id is provided', async () => {
      const res = await request(app)
        .post('/api/vnc/sessions')
        .send({ vnc_port: 5900 });
      expect(res.status).toBe(400);
      expect(res.body.error).toBeDefined();
    });

    it('returns 400 with empty body', async () => {
      const res = await request(app)
        .post('/api/vnc/sessions')
        .send({});
      expect(res.status).toBe(400);
    });

    it('created session has state connecting', async () => {
      const res = await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc.example.com' });
      expect(res.status).toBe(201);
      expect(res.body.state).toBe('connecting');
    });

    it('created session uses default vnc_port 5900 when not specified', async () => {
      const res = await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc.example.com' });
      expect(res.status).toBe(201);
      expect(res.body.vnc_port).toBe(5900);
    });
  });

  // ---------------------------------------------------------------------------
  // GET /api/vnc/sessions/:id
  // ---------------------------------------------------------------------------

  describe('GET /api/vnc/sessions/:id', () => {
    it('returns 200 with the session when owner matches', async () => {
      const created = await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc.example.com' });
      const res = await request(app).get(`/api/vnc/sessions/${created.body.id}`);
      expect(res.status).toBe(200);
      expect(res.body.id).toBe(created.body.id);
    });

    it('returns 404 for unknown session id', async () => {
      const res = await request(app).get('/api/vnc/sessions/nonexistent');
      expect(res.status).toBe(404);
    });

    it('returns 404 for wrong owner', async () => {
      // Create a session directly on the broker owned by 'alice'
      const session = await vncBroker.create({ hostname: 'vnc.example.com' }, 'alice');

      // Request arrives as 'anonymous' (auth mode none), not 'alice'
      const res = await request(app).get(`/api/vnc/sessions/${session.id}`);
      expect(res.status).toBe(404);
    });
  });

  // ---------------------------------------------------------------------------
  // PATCH /api/vnc/sessions/:id
  // ---------------------------------------------------------------------------

  describe('PATCH /api/vnc/sessions/:id', () => {
    it('returns 200 with updated session when valid row/col provided', async () => {
      const created = await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc.example.com' });
      const res = await request(app)
        .patch(`/api/vnc/sessions/${created.body.id}`)
        .send({ row: 1, col: 2 });
      expect(res.status).toBe(200);
      expect(res.body.row).toBe(1);
      expect(res.body.col).toBe(2);
    });

    it('returns 400 when row is missing', async () => {
      const created = await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc.example.com' });
      const res = await request(app)
        .patch(`/api/vnc/sessions/${created.body.id}`)
        .send({ col: 2 });
      expect(res.status).toBe(400);
    });

    it('returns 400 when col is missing', async () => {
      const created = await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc.example.com' });
      const res = await request(app)
        .patch(`/api/vnc/sessions/${created.body.id}`)
        .send({ row: 1 });
      expect(res.status).toBe(400);
    });

    it('returns 400 with empty body', async () => {
      const created = await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc.example.com' });
      const res = await request(app)
        .patch(`/api/vnc/sessions/${created.body.id}`)
        .send({});
      expect(res.status).toBe(400);
    });

    it('returns 400 for negative row value', async () => {
      const created = await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc.example.com' });
      const res = await request(app)
        .patch(`/api/vnc/sessions/${created.body.id}`)
        .send({ row: -1, col: 0 });
      expect(res.status).toBe(400);
    });

    it('returns 404 for unknown session id', async () => {
      const res = await request(app)
        .patch('/api/vnc/sessions/nonexistent')
        .send({ row: 0, col: 0 });
      expect(res.status).toBe(404);
    });

    it('returns 404 when patching a session owned by a different owner', async () => {
      const session = await vncBroker.create({ hostname: 'vnc.example.com' }, 'alice');
      const res = await request(app)
        .patch(`/api/vnc/sessions/${session.id}`)
        .send({ row: 0, col: 0 });
      expect(res.status).toBe(404);
    });
  });

  // ---------------------------------------------------------------------------
  // DELETE /api/vnc/sessions/:id
  // ---------------------------------------------------------------------------

  describe('DELETE /api/vnc/sessions/:id', () => {
    it('returns 204 on successful deletion', async () => {
      const created = await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc.example.com' });
      const res = await request(app).delete(`/api/vnc/sessions/${created.body.id}`);
      expect(res.status).toBe(204);
    });

    it('session is no longer returned after deletion', async () => {
      const created = await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc.example.com' });
      await request(app).delete(`/api/vnc/sessions/${created.body.id}`);
      const res = await request(app).get(`/api/vnc/sessions/${created.body.id}`);
      expect(res.status).toBe(404);
    });

    it('returns 404 for unknown session id', async () => {
      const res = await request(app).delete('/api/vnc/sessions/nonexistent');
      expect(res.status).toBe(404);
    });

    it('returns 404 when deleting a session owned by a different owner', async () => {
      const session = await vncBroker.create({ hostname: 'vnc.example.com' }, 'alice');
      const res = await request(app).delete(`/api/vnc/sessions/${session.id}`);
      expect(res.status).toBe(404);
    });
  });

  // ---------------------------------------------------------------------------
  // POST /api/vnc/sessions/:id/reconnect
  // ---------------------------------------------------------------------------

  describe('POST /api/vnc/sessions/:id/reconnect', () => {
    it('returns 200 with session state set to connecting', async () => {
      const created = await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc.example.com' });

      // Simulate session going disconnected
      vncBroker.setState(created.body.id, 'disconnected');
      expect(vncBroker.get(created.body.id).state).toBe('disconnected');

      const res = await request(app).post(`/api/vnc/sessions/${created.body.id}/reconnect`);
      expect(res.status).toBe(200);
      expect(res.body.state).toBe('connecting');
    });

    it('returns 200 from error state, setting state to connecting', async () => {
      const created = await request(app)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc.example.com' });

      vncBroker.setState(created.body.id, 'error');

      const res = await request(app).post(`/api/vnc/sessions/${created.body.id}/reconnect`);
      expect(res.status).toBe(200);
      expect(res.body.state).toBe('connecting');
    });

    it('returns 404 for unknown session id', async () => {
      const res = await request(app).post('/api/vnc/sessions/nonexistent/reconnect');
      expect(res.status).toBe(404);
    });

    it('returns 404 when reconnecting a session owned by a different owner', async () => {
      const session = await vncBroker.create({ hostname: 'vnc.example.com' }, 'alice');
      const res = await request(app).post(`/api/vnc/sessions/${session.id}/reconnect`);
      expect(res.status).toBe(404);
    });
  });

  // ---------------------------------------------------------------------------
  // Auth mode: local — 401 without token
  // ---------------------------------------------------------------------------

  describe('auth mode: local', () => {
    let localApp: express.Express;

    beforeEach(async () => {
      const configDir = path.join(tmpDir, 'config');
      fs.writeFileSync(
        path.join(configDir, 'auth.yaml'),
        'auth:\n  mode: local\n  users: []\n'
      );

      jest.resetModules();
      const { default: vncRouter2 } = require('@backend/api/vnc');
      const vncBroker2 = require('@backend/services/vncBroker').vncBroker;
      await vncBroker2.initialize();

      localApp = express();
      localApp.use(express.json());
      localApp.use('/api/vnc', vncRouter2);
    });

    it('returns 401 on GET /api/vnc/sessions without token', async () => {
      const res = await request(localApp).get('/api/vnc/sessions');
      expect(res.status).toBe(401);
    });

    it('returns 401 on POST /api/vnc/sessions without token', async () => {
      const res = await request(localApp)
        .post('/api/vnc/sessions')
        .send({ hostname: 'vnc.example.com' });
      expect(res.status).toBe(401);
    });

    it('returns 200 on GET /api/vnc/sessions with valid Bearer token', async () => {
      const { signToken } = require('@backend/middleware/auth');
      const token = signToken('alice');
      const res = await request(localApp)
        .get('/api/vnc/sessions')
        .set('Authorization', `Bearer ${token}`);
      expect(res.status).toBe(200);
      expect(res.body).toEqual([]);
    });
  });
});
