import { Request, Response, NextFunction } from 'express';
import jwt from 'jsonwebtoken';
import * as crypto from 'crypto';
import { persistence } from '../services/persistenceManager';
import { AuthUser } from '../types';

const TOKEN_TTL = '8h';

let cachedSecret: string | null = null;

// Resolves the JWT signing secret in this order:
//   1. JWT_SECRET env var
//   2. auth.yaml's `jwt_secret` field (generated and persisted on first read if absent)
//   3. ephemeral random (only if auth.yaml is unavailable, e.g. pre-bootstrap)
// Tokens issued under (3) won't survive a restart, which is acceptable for the
// brief pre-bootstrap window.
function getJwtSecret(): string {
  if (cachedSecret) return cachedSecret;

  if (process.env.JWT_SECRET) {
    cachedSecret = process.env.JWT_SECRET;
    return cachedSecret;
  }

  if (process.env.NODE_ENV === 'test') {
    cachedSecret = 'test-secret';
    return cachedSecret;
  }

  try {
    const cfg = persistence.loadAuth();
    if (cfg.auth.jwt_secret) {
      cachedSecret = cfg.auth.jwt_secret;
      return cachedSecret;
    }
    const generated = crypto.randomBytes(32).toString('hex');
    cfg.auth.jwt_secret = generated;
    persistence.saveAuth(cfg);
    cachedSecret = generated;
    console.log('Generated and saved new JWT signing secret to auth.yaml');
    return cachedSecret;
  } catch {
    cachedSecret = crypto.randomBytes(32).toString('hex');
    console.warn('JWT secret could not be persisted; using ephemeral random. Tokens will be invalidated on restart.');
    return cachedSecret;
  }
}

export interface AuthPayload {
  sub: string;
  iat: number;
}

export function signToken(username: string): string {
  return jwt.sign({ sub: username }, getJwtSecret(), { expiresIn: TOKEN_TTL });
}

export function verifyToken(token: string): AuthPayload {
  return jwt.verify(token, getJwtSecret()) as AuthPayload;
}

export function requireAuth(req: Request, res: Response, next: NextFunction): void {
  try {
    const authConfig = persistence.loadAuth();
    if (authConfig.auth.mode === 'none') {
      next();
      return;
    }
  } catch {
    // Default to requiring auth if config can't be loaded
  }

  const authHeader = req.headers['authorization'];
  const token = authHeader?.startsWith('Bearer ') ? authHeader.slice(7) : undefined;

  if (!token) {
    res.status(401).json({ error: 'Unauthorized' });
    return;
  }

  try {
    const payload = verifyToken(token);
    (req as Request & { user?: AuthPayload }).user = payload;
    next();
  } catch {
    res.status(401).json({ error: 'Invalid or expired token' });
  }
}

// Resolves whether a username is an admin (owner). A user is an admin when their
// entry has `admin: true`. For backward compatibility with configs created before
// the admin flag existed, if NO user carries an explicit admin flag the first user
// in the list is treated as the admin/owner.
export function resolveIsAdmin(users: AuthUser[], username: string): boolean {
  if (users.length === 0) return false;
  const anyExplicitAdmin = users.some(u => u.admin === true);
  if (!anyExplicitAdmin) {
    return users[0].username === username;
  }
  const user = users.find(u => u.username === username);
  return user?.admin === true;
}

// Requires an authenticated admin. Must run after requireAuth. In mode 'none'
// there are no accounts to manage, so access is denied.
export function requireAdmin(req: Request, res: Response, next: NextFunction): void {
  const payload = (req as Request & { user?: AuthPayload }).user;
  const username = payload?.sub;
  if (!username) {
    res.status(401).json({ error: 'Unauthorized' });
    return;
  }
  try {
    const authConfig = persistence.loadAuth();
    if (authConfig.auth.mode === 'none') {
      res.status(403).json({ error: 'User management is unavailable when auth is disabled' });
      return;
    }
    if (!resolveIsAdmin(authConfig.auth.users || [], username)) {
      res.status(403).json({ error: 'Admin privileges required' });
      return;
    }
    next();
  } catch {
    res.status(500).json({ error: 'Internal server error' });
  }
}

export function requireAuthWs(token: string | undefined): boolean {
  try {
    const authConfig = persistence.loadAuth();
    if (authConfig.auth.mode === 'none') return true;
  } catch {
    // Default to requiring auth
  }

  if (!token) return false;
  try {
    verifyToken(token);
    return true;
  } catch {
    return false;
  }
}
