import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

describe('Auth Middleware', () => {
  let tmpDir: string;
  let originalHome: string | undefined;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-auth-'));
    originalHome = process.env.WEBMUX_HOME;
    process.env.WEBMUX_HOME = tmpDir;

    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });

    jest.resetModules();
  });

  afterEach(() => {
    if (originalHome === undefined) {
      delete process.env.WEBMUX_HOME;
    } else {
      process.env.WEBMUX_HOME = originalHome;
    }
    fs.rmSync(tmpDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
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

  describe('resolveIsAdmin', () => {
    it('respects explicit admin flags', () => {
      const { resolveIsAdmin } = require('@backend/middleware/auth');
      const users = [
        { username: 'a', password_hash: 'x', admin: true },
        { username: 'b', password_hash: 'x', admin: false },
      ];
      expect(resolveIsAdmin(users, 'a')).toBe(true);
      expect(resolveIsAdmin(users, 'b')).toBe(false);
      expect(resolveIsAdmin(users, 'missing')).toBe(false);
    });

    it('falls back to the first user when no explicit admin flags exist (legacy config)', () => {
      const { resolveIsAdmin } = require('@backend/middleware/auth');
      const users = [
        { username: 'first', password_hash: 'x' },
        { username: 'second', password_hash: 'x' },
      ];
      expect(resolveIsAdmin(users, 'first')).toBe(true);
      expect(resolveIsAdmin(users, 'second')).toBe(false);
    });

    it('returns false for an empty user list', () => {
      const { resolveIsAdmin } = require('@backend/middleware/auth');
      expect(resolveIsAdmin([], 'anyone')).toBe(false);
    });
  });

  describe('requireAdmin', () => {
    it('denies user management in none mode', () => {
      writeAuthConfig('none');
      const { requireAdmin } = require('@backend/middleware/auth');
      const req = { user: { sub: 'admin' } } as any;
      const res = { status: jest.fn().mockReturnThis(), json: jest.fn() } as any;
      const next = jest.fn();
      requireAdmin(req, res, next);
      expect(res.status).toHaveBeenCalledWith(403);
      expect(next).not.toHaveBeenCalled();
    });

    it('returns 401 when unauthenticated', () => {
      writeAuthConfig('local');
      const { requireAdmin } = require('@backend/middleware/auth');
      const req = {} as any;
      const res = { status: jest.fn().mockReturnThis(), json: jest.fn() } as any;
      const next = jest.fn();
      requireAdmin(req, res, next);
      expect(res.status).toHaveBeenCalledWith(401);
    });

    it('allows an admin and blocks a non-admin', () => {
      fs.writeFileSync(
        path.join(tmpDir, 'config', 'auth.yaml'),
        'auth:\n  mode: local\n  users:\n' +
        '    - username: boss\n      password_hash: x\n      admin: true\n' +
        '    - username: guest\n      password_hash: x\n      admin: false\n',
      );
      const { requireAdmin } = require('@backend/middleware/auth');

      const okNext = jest.fn();
      const okRes = { status: jest.fn().mockReturnThis(), json: jest.fn() } as any;
      requireAdmin({ user: { sub: 'boss' } } as any, okRes, okNext);
      expect(okNext).toHaveBeenCalled();

      const denyNext = jest.fn();
      const denyRes = { status: jest.fn().mockReturnThis(), json: jest.fn() } as any;
      requireAdmin({ user: { sub: 'guest' } } as any, denyRes, denyNext);
      expect(denyRes.status).toHaveBeenCalledWith(403);
      expect(denyNext).not.toHaveBeenCalled();
    });
  });
});
