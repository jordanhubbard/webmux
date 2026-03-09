import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

describe('Auth Middleware', () => {
  let tmpDir: string;
  let originalRoot: string | undefined;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-auth-'));
    originalRoot = process.env.WEBMUX_ROOT;
    process.env.WEBMUX_ROOT = tmpDir;

    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });

    jest.resetModules();
  });

  afterEach(() => {
    if (originalRoot === undefined) {
      delete process.env.WEBMUX_ROOT;
    } else {
      process.env.WEBMUX_ROOT = originalRoot;
    }
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  function writeAuthConfig(mode: string) {
    fs.writeFileSync(
      path.join(tmpDir, 'config', 'auth.yaml'),
      `auth:\n  mode: ${mode}\n  users: []\n`
    );
  }

  it('signToken and verifyToken round-trip', () => {
    writeAuthConfig('local');
    const { signToken, verifyToken } = require('@backend/middleware/auth');
    const token = signToken('admin');
    const payload = verifyToken(token);
    expect(payload.sub).toBe('admin');
  });

  it('requireAuth passes through in none mode', () => {
    writeAuthConfig('none');
    const { requireAuth } = require('@backend/middleware/auth');
    const req = { headers: {} } as any;
    const res = { status: jest.fn().mockReturnThis(), json: jest.fn() } as any;
    const next = jest.fn();
    requireAuth(req, res, next);
    expect(next).toHaveBeenCalled();
  });

  it('requireAuth returns 401 without token in local mode', () => {
    writeAuthConfig('local');
    const { requireAuth } = require('@backend/middleware/auth');
    const req = { headers: {} } as any;
    const res = { status: jest.fn().mockReturnThis(), json: jest.fn() } as any;
    const next = jest.fn();
    requireAuth(req, res, next);
    expect(res.status).toHaveBeenCalledWith(401);
    expect(next).not.toHaveBeenCalled();
  });

  it('requireAuth accepts valid Bearer token', () => {
    writeAuthConfig('local');
    const { requireAuth, signToken } = require('@backend/middleware/auth');
    const token = signToken('admin');
    const req = { headers: { authorization: `Bearer ${token}` } } as any;
    const res = { status: jest.fn().mockReturnThis(), json: jest.fn() } as any;
    const next = jest.fn();
    requireAuth(req, res, next);
    expect(next).toHaveBeenCalled();
    expect(req.user.sub).toBe('admin');
  });

  it('requireAuth rejects invalid token', () => {
    writeAuthConfig('local');
    const { requireAuth } = require('@backend/middleware/auth');
    const req = { headers: { authorization: 'Bearer invalid.token.here' } } as any;
    const res = { status: jest.fn().mockReturnThis(), json: jest.fn() } as any;
    const next = jest.fn();
    requireAuth(req, res, next);
    expect(res.status).toHaveBeenCalledWith(401);
  });

  it('requireAuthWs returns true in none mode', () => {
    writeAuthConfig('none');
    const { requireAuthWs } = require('@backend/middleware/auth');
    expect(requireAuthWs(undefined)).toBe(true);
  });

  it('requireAuthWs returns false without token in local mode', () => {
    writeAuthConfig('local');
    const { requireAuthWs } = require('@backend/middleware/auth');
    expect(requireAuthWs(undefined)).toBe(false);
  });

  it('requireAuthWs accepts valid token', () => {
    writeAuthConfig('local');
    const { requireAuthWs, signToken } = require('@backend/middleware/auth');
    const token = signToken('admin');
    expect(requireAuthWs(token)).toBe(true);
  });

  it('requireAuthWs rejects invalid token', () => {
    writeAuthConfig('local');
    const { requireAuthWs } = require('@backend/middleware/auth');
    expect(requireAuthWs('bad-token')).toBe(false);
  });
});
