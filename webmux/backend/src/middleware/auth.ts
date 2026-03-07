import { Request, Response, NextFunction } from 'express';
import jwt from 'jsonwebtoken';
import { persistence } from '../services/persistenceManager';

const JWT_SECRET = process.env.JWT_SECRET || 'webmux-dev-secret-change-in-production';
const TOKEN_TTL = '8h';

if (!process.env.JWT_SECRET && process.env.NODE_ENV !== 'test') {
  console.warn('WARNING: JWT_SECRET is not set. Using insecure default. Set JWT_SECRET in production.');
}

export interface AuthPayload {
  sub: string;
  iat: number;
}

export function signToken(username: string): string {
  return jwt.sign({ sub: username }, JWT_SECRET, { expiresIn: TOKEN_TTL });
}

export function verifyToken(token: string): AuthPayload {
  return jwt.verify(token, JWT_SECRET) as AuthPayload;
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
  const token = authHeader?.startsWith('Bearer ') ? authHeader.slice(7) : (req as Request & { cookies?: Record<string, string> }).cookies?.token;

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
