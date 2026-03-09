import { Router, Request, Response } from 'express';
import rateLimit from 'express-rate-limit';
import argon2 from 'argon2';
import { persistence } from '../services/persistenceManager';
import { signToken, requireAuth, AuthPayload } from '../middleware/auth';

const router = Router();

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
    authConfig.auth.users = [{ username, password_hash: hash }];
    persistence.saveAuth(authConfig);

    const token = signToken(username);
    persistence.appendEvent({ type: 'bootstrap_complete', username });
    res.json({ token, mode: 'local' });
  } catch (err) {
    console.error('Bootstrap error:', err);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.post('/register', requireAuth, async (req: Request, res: Response) => {
  const { username, password } = req.body as { username?: string; password?: string };

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
    users.push({ username, password_hash: hash });
    authConfig.auth.users = users;
    persistence.saveAuth(authConfig);

    persistence.appendEvent({ type: 'account_created', username, created_by: (req as Request & { user?: AuthPayload }).user?.sub });
    res.status(201).json({ username });
  } catch (err) {
    console.error('Register error:', err);
    res.status(500).json({ error: 'Internal server error' });
  }
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
