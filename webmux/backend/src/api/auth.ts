import { Router, Request, Response } from 'express';
import rateLimit from 'express-rate-limit';
import argon2 from 'argon2';
import * as crypto from 'crypto';
import { persistence } from '../services/persistenceManager';
import { signToken, requireAuth, requireAdmin, resolveIsAdmin, AuthPayload } from '../middleware/auth';

const router = Router();

// One-time WebSocket tickets. The browser exchanges its bearer token for a
// short-lived single-use ticket via POST /api/auth/ticket, then passes the
// ticket in the WS upgrade URL. Tickets never appear in HTTP access logs for
// the long-lived auth token, and are invalidated after one use.
const TICKET_TTL_MS = 60_000;
const tickets = new Map<string, { username: string; expires: number }>();

function purgeExpiredTickets(): void {
  const now = Date.now();
  for (const [id, t] of tickets) {
    if (t.expires < now) tickets.delete(id);
  }
}

export function consumeTicket(ticket: string): string | null {
  purgeExpiredTickets();
  const t = tickets.get(ticket);
  if (!t) return null;
  tickets.delete(ticket);
  if (t.expires < Date.now()) return null;
  return t.username;
}

const authLimiter = rateLimit({
  windowMs: 15 * 60 * 1000,
  max: 10,
  standardHeaders: true,
  legacyHeaders: false,
  message: { error: 'Too many requests, please try again later.' },
});

router.use('/login', authLimiter);
router.use('/bootstrap', authLimiter);
router.use('/register', authLimiter);

router.post('/login', async (req: Request, res: Response) => {
  const { username, password } = req.body as { username?: string; password?: string };

  if (!username || !password) {
    res.status(400).json({ error: 'Username and password required' });
    return;
  }

  try {
    const authConfig = persistence.loadAuth();

    if (authConfig.auth.mode === 'none') {
      const token = signToken(username);
      res.json({ token, mode: 'none' });
      return;
    }

    const users = authConfig.auth.users || [];
    const user = users.find(u => u.username === username);
    if (!user) {
      persistence.appendEvent({ type: 'login_failed', username });
      res.status(401).json({ error: 'Invalid credentials' });
      return;
    }

    const valid = await argon2.verify(user.password_hash, password);
    if (!valid) {
      persistence.appendEvent({ type: 'login_failed', username });
      res.status(401).json({ error: 'Invalid credentials' });
      return;
    }

    const token = signToken(username);
    persistence.appendEvent({ type: 'login_success', username });
    res.json({ token, mode: 'local' });
  } catch (err) {
    console.error('Login error:', err);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.post('/bootstrap', async (req: Request, res: Response) => {
  const { username, password } = req.body as { username?: string; password?: string };

  if (!username || !password) {
    res.status(400).json({ error: 'Username and password required' });
    return;
  }

  try {
    const authConfig = persistence.loadAuth();
    const users = authConfig.auth.users || [];

    if (users.length > 0) {
      res.status(403).json({ error: 'Bootstrap not available — accounts already exist' });
      return;
    }

    const hash = await argon2.hash(password, { type: argon2.argon2id });
    // The first account is the owner and is granted admin privileges.
    authConfig.auth.users = [{ username, password_hash: hash, admin: true }];
    persistence.saveAuth(authConfig);

    const token = signToken(username);
    persistence.appendEvent({ type: 'bootstrap_complete', username });
    res.json({ token, mode: 'local' });
  } catch (err) {
    console.error('Bootstrap error:', err);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.post('/refresh', requireAuth, (req: Request, res: Response) => {
  const username = (req as Request & { user?: AuthPayload }).user?.sub || 'anonymous';
  res.json({ token: signToken(username), mode: username === 'anonymous' ? 'none' : 'local' });
});

// Creating accounts is restricted to admins (owners).
router.post('/register', requireAuth, requireAdmin, async (req: Request, res: Response) => {
  const { username, password, admin } = req.body as { username?: string; password?: string; admin?: boolean };

  if (!username || !password) {
    res.status(400).json({ error: 'Username and password required' });
    return;
  }

  if (username.length < 2 || username.length > 64) {
    res.status(400).json({ error: 'Username must be 2-64 characters' });
    return;
  }

  if (password.length < 4) {
    res.status(400).json({ error: 'Password must be at least 4 characters' });
    return;
  }

  try {
    const authConfig = persistence.loadAuth();
    const users = authConfig.auth.users || [];

    if (users.some(u => u.username === username)) {
      res.status(409).json({ error: 'Username already exists' });
      return;
    }

    const hash = await argon2.hash(password, { type: argon2.argon2id });
    users.push({ username, password_hash: hash, admin: admin === true });
    authConfig.auth.users = users;
    persistence.saveAuth(authConfig);

    persistence.appendEvent({ type: 'account_created', username, admin: admin === true, created_by: (req as Request & { user?: AuthPayload }).user?.sub });
    res.status(201).json({ username, admin: admin === true });
  } catch (err) {
    console.error('Register error:', err);
    res.status(500).json({ error: 'Internal server error' });
  }
});

// Returns the current authenticated user and whether they are an admin. The UI
// uses this to decide whether to surface user-management controls.
router.get('/me', requireAuth, (req: Request, res: Response) => {
  const username = (req as Request & { user?: AuthPayload }).user?.sub;
  try {
    const authConfig = persistence.loadAuth();
    if (authConfig.auth.mode === 'none') {
      res.json({ username: username || 'anonymous', admin: false });
      return;
    }
    if (!username) {
      res.status(401).json({ error: 'Unauthorized' });
      return;
    }
    res.json({ username, admin: resolveIsAdmin(authConfig.auth.users || [], username) });
  } catch {
    res.status(500).json({ error: 'Internal server error' });
  }
});

// Lists all accounts (admin only). Password hashes are never returned.
router.get('/users', requireAuth, requireAdmin, (_req: Request, res: Response) => {
  try {
    const authConfig = persistence.loadAuth();
    const users = authConfig.auth.users || [];
    res.json(users.map(u => ({ username: u.username, admin: resolveIsAdmin(users, u.username) })));
  } catch {
    res.status(500).json({ error: 'Internal server error' });
  }
});

// Removes an account (admin only). An admin cannot remove their own account,
// which also guarantees at least one admin always remains.
router.delete('/users/:username', requireAuth, requireAdmin, (req: Request, res: Response) => {
  const target = req.params.username;
  const actor = (req as Request & { user?: AuthPayload }).user?.sub;

  if (target === actor) {
    res.status(400).json({ error: 'You cannot remove your own account' });
    return;
  }

  try {
    const authConfig = persistence.loadAuth();
    const users = authConfig.auth.users || [];
    const user = users.find(u => u.username === target);
    if (!user) {
      res.status(404).json({ error: 'User not found' });
      return;
    }

    authConfig.auth.users = users.filter(u => u.username !== target);
    persistence.saveAuth(authConfig);

    persistence.appendEvent({ type: 'account_deleted', username: target, deleted_by: actor });
    res.status(204).end();
  } catch (err) {
    console.error('Delete user error:', err);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.post('/ticket', requireAuth, (req: Request, res: Response) => {
  purgeExpiredTickets();
  const payload = (req as Request & { user?: AuthPayload }).user;
  // requireAuth guarantees payload exists for mode: local; in mode: none we
  // record 'anonymous' so the consumer still gets a usable owner.
  const username = payload?.sub || 'anonymous';
  const ticket = crypto.randomBytes(24).toString('hex');
  tickets.set(ticket, { username, expires: Date.now() + TICKET_TTL_MS });
  res.json({ ticket, expires_in: Math.floor(TICKET_TTL_MS / 1000) });
});

router.get('/status', (_req: Request, res: Response) => {
  try {
    const authConfig = persistence.loadAuth();
    const users = authConfig.auth.users || [];
    res.json({
      mode: authConfig.auth.mode,
      bootstrap_required: users.length === 0,
    });
  } catch {
    res.json({ mode: 'local', bootstrap_required: true });
  }
});

export default router;
