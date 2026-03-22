import { Router, Request, Response } from 'express';
import { sessionBroker } from '../services/sessionBroker';
import { requireAuth, AuthPayload } from '../middleware/auth';
import { CreateSessionRequest } from '../types';

const router = Router();
router.use(requireAuth);

function getOwner(req: Request): string {
  return (req as Request & { user?: AuthPayload }).user?.sub || 'anonymous';
}

router.get('/', (req: Request, res: Response) => {
  const owner = getOwner(req);
  res.json(sessionBroker.listByOwner(owner));
});

router.get('/:id', (req: Request, res: Response) => {
  const owner = getOwner(req);
  const session = sessionBroker.get(req.params.id);
  if (!session || session.owner !== owner) {
    res.status(404).json({ error: 'Session not found' });
    return;
  }
  res.json(session);
});

router.post('/', async (req: Request, res: Response) => {
  try {
    const body = req.body as CreateSessionRequest;
    // Claude sessions don't require username; all others do
    if (!body.username && body.session_type !== 'claude') {
      res.status(400).json({ error: 'username is required' });
      return;
    }
    const owner = getOwner(req);
    const session = await sessionBroker.create(body, owner);
    res.status(201).json(session);
  } catch (err) {
    console.error('Create session error:', err);
    res.status(500).json({ error: 'Failed to create session' });
  }
});

router.post('/:id/reconnect', async (req: Request, res: Response) => {
  try {
    const owner = getOwner(req);
    const session = sessionBroker.get(req.params.id);
    if (!session || session.owner !== owner) {
      res.status(404).json({ error: 'Session not found' });
      return;
    }
    const { password } = req.body as { password?: string };
    const updated = await sessionBroker.reconnect(req.params.id, password);
    res.json(updated);
  } catch (err) {
    console.error('Reconnect error:', err);
    res.status(500).json({ error: 'Failed to reconnect session' });
  }
});

router.patch('/:id', (req: Request, res: Response) => {
  const owner = getOwner(req);
  const session = sessionBroker.get(req.params.id);
  if (!session || session.owner !== owner) {
    res.status(404).json({ error: 'Session not found' });
    return;
  }
  const { row, col, title } = req.body as { row?: number; col?: number; title?: string };

  if (title !== undefined) {
    const trimmed = title.trim();
    if (trimmed.length === 0 || trimmed.length > 128) {
      res.status(400).json({ error: 'title must be 1-128 characters' });
      return;
    }
    const updated = sessionBroker.rename(req.params.id, trimmed);
    res.json(updated);
    return;
  }

  if (row === undefined || col === undefined) {
    res.status(400).json({ error: 'row and col are required' });
    return;
  }
  const updated = sessionBroker.move(req.params.id, row, col);
  res.json(updated);
});

router.delete('/:id', async (req: Request, res: Response) => {
  try {
    const owner = getOwner(req);
    const session = sessionBroker.get(req.params.id);
    if (!session || session.owner !== owner) {
      res.status(404).json({ error: 'Session not found' });
      return;
    }
    await sessionBroker.delete(req.params.id);
    res.status(204).send();
  } catch (err) {
    console.error('Delete session error:', err);
    res.status(500).json({ error: 'Failed to delete session' });
  }
});

export default router;
