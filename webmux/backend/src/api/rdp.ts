import { Router, Request, Response } from 'express';
import { rdpBroker } from '../services/rdpBroker';
import { requireAuth, AuthPayload } from '../middleware/auth';
import { CreateRdpSessionRequest } from '../types';

const router = Router();
router.use(requireAuth);

function getOwner(req: Request): string {
  return (req as Request & { user?: AuthPayload }).user?.sub || 'anonymous';
}

router.get('/sessions', (req: Request, res: Response) => {
  const owner = getOwner(req);
  res.json(rdpBroker.listByOwner(owner));
});

router.get('/sessions/:id', (req: Request, res: Response) => {
  const owner = getOwner(req);
  const session = rdpBroker.get(req.params.id);
  if (!session || session.owner !== owner) {
    res.status(404).json({ error: 'Session not found' });
    return;
  }
  res.json(session);
});

router.post('/sessions', async (req: Request, res: Response) => {
  try {
    const body = req.body as CreateRdpSessionRequest;
    if (!body.hostname && !body.host_id) {
      res.status(400).json({ error: 'hostname or host_id is required' });
      return;
    }
    const owner = getOwner(req);
    const session = await rdpBroker.create(body, owner);
    res.status(201).json(session);
  } catch (err) {
    console.error('Create RDP session error:', err);
    const msg = err instanceof Error ? err.message : 'Failed to create RDP session';
    res.status(500).json({ error: msg });
  }
});

router.patch('/sessions/:id', (req: Request, res: Response) => {
  const owner = getOwner(req);
  const session = rdpBroker.get(req.params.id);
  if (!session || session.owner !== owner) {
    res.status(404).json({ error: 'Session not found' });
    return;
  }
  const { row, col } = req.body as { row?: number; col?: number };
  if (row === undefined || col === undefined) {
    res.status(400).json({ error: 'row and col are required' });
    return;
  }
  if (typeof row !== 'number' || typeof col !== 'number' || row < 0 || col < 0) {
    res.status(400).json({ error: 'row and col must be non-negative numbers' });
    return;
  }
  const updated = rdpBroker.move(req.params.id, row, col);
  res.json(updated);
});

router.post('/sessions/:id/reconnect', (req: Request, res: Response) => {
  const owner = getOwner(req);
  const session = rdpBroker.get(req.params.id);
  if (!session || session.owner !== owner) {
    res.status(404).json({ error: 'Session not found' });
    return;
  }
  rdpBroker.setState(req.params.id, 'connecting');
  const updated = rdpBroker.get(req.params.id);
  res.json(updated);
});

router.delete('/sessions/:id', async (req: Request, res: Response) => {
  try {
    const owner = getOwner(req);
    const session = rdpBroker.get(req.params.id);
    if (!session || session.owner !== owner) {
      res.status(404).json({ error: 'Session not found' });
      return;
    }
    await rdpBroker.delete(req.params.id);
    res.status(204).send();
  } catch (err) {
    console.error('Delete RDP session error:', err);
    res.status(500).json({ error: 'Failed to delete RDP session' });
  }
});

export default router;
