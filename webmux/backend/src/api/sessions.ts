import { Router, Request, Response } from 'express';
import { sessionBroker } from '../services/sessionBroker';
import { requireAuth } from '../middleware/auth';
import { CreateSessionRequest } from '../types';

const router = Router();
router.use(requireAuth);

router.get('/', (_req: Request, res: Response) => {
  res.json(sessionBroker.list());
});

router.get('/:id', (req: Request, res: Response) => {
  const session = sessionBroker.get(req.params.id);
  if (!session) {
    res.status(404).json({ error: 'Session not found' });
    return;
  }
  res.json(session);
});

router.post('/', async (req: Request, res: Response) => {
  try {
    const body = req.body as CreateSessionRequest;
    if (!body.username) {
      res.status(400).json({ error: 'username is required' });
      return;
    }
    const session = await sessionBroker.create(body);
    res.status(201).json(session);
  } catch (err) {
    console.error('Create session error:', err);
    res.status(500).json({ error: String(err) });
  }
});

router.post('/:id/reconnect', async (req: Request, res: Response) => {
  try {
    const { password } = req.body as { password?: string };
    const session = await sessionBroker.reconnect(req.params.id, password);
    res.json(session);
  } catch (err) {
    res.status(500).json({ error: String(err) });
  }
});

router.post('/:id/split-right', (req: Request, res: Response) => {
  try {
    const pos = sessionBroker.splitRight(req.params.id);
    res.json(pos);
  } catch (err) {
    res.status(404).json({ error: String(err) });
  }
});

router.post('/:id/split-below', (req: Request, res: Response) => {
  try {
    const pos = sessionBroker.splitBelow(req.params.id);
    res.json(pos);
  } catch (err) {
    res.status(404).json({ error: String(err) });
  }
});

router.delete('/:id', async (req: Request, res: Response) => {
  try {
    await sessionBroker.delete(req.params.id);
    res.status(204).send();
  } catch (err) {
    res.status(500).json({ error: String(err) });
  }
});

export default router;
