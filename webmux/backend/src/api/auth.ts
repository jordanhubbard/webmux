import { Router, Request, Response } from 'express';
import argon2 from 'argon2';
import { persistence } from '../services/persistenceManager';
import { signToken } from '../middleware/auth';

const router = Router();

router.post('/login', async (req: Request, res: Response) => {
  const { username, password } = req.body as { username?: string; password?: string };

  if (!username || !password) {
    res.status(400).json({ error: 'Username and password required' });
    return;
  }

  try {
    const authConfig = persistence.loadAuth();

    if (authConfig.auth.mode === 'none') {
      // Trusted mode - no auth needed but still return a token
      const token = signToken(username);
      res.json({ token, mode: 'none' });
      return;
    }

    if (authConfig.auth.bootstrap_required) {
      res.status(403).json({ error: 'Bootstrap required', bootstrap_required: true });
      return;
    }

    const valid = await argon2.verify(authConfig.auth.password_hash, password);
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
    if (!authConfig.auth.bootstrap_required) {
      res.status(403).json({ error: 'Bootstrap not required' });
      return;
    }

    const hash = await argon2.hash(password, { type: argon2.argon2id });
    authConfig.auth.password_hash = hash;
    authConfig.auth.bootstrap_required = false;
    persistence.saveAuth(authConfig);

    const token = signToken(username);
    persistence.appendEvent({ type: 'bootstrap_complete', username });
    res.json({ token, mode: 'local' });
  } catch (err) {
    console.error('Bootstrap error:', err);
    res.status(500).json({ error: 'Internal server error' });
  }
});

router.get('/status', (req: Request, res: Response) => {
  try {
    const authConfig = persistence.loadAuth();
    res.json({
      mode: authConfig.auth.mode,
      bootstrap_required: authConfig.auth.bootstrap_required,
    });
  } catch {
    res.json({ mode: 'local', bootstrap_required: true });
  }
});

export default router;
