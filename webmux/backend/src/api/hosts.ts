import { Router, Request, Response } from 'express';
import { v4 as uuidv4 } from 'uuid';
import { persistence } from '../services/persistenceManager';
import { requireAuth } from '../middleware/auth';
import { HostEntry } from '../types';

const router = Router();
router.use(requireAuth);

router.get('/', (_req: Request, res: Response) => {
  try {
    const config = persistence.loadHosts();
    res.json(config.hosts);
  } catch {
    res.status(500).json({ error: 'Failed to load hosts' });
  }
});

router.post('/', (req: Request, res: Response) => {
  const { hostname, port, tags, mosh_allowed, id } = req.body as Partial<HostEntry>;

  if (!hostname) {
    res.status(400).json({ error: 'hostname is required' });
    return;
  }

  try {
    const config = persistence.loadHosts();
    const host: HostEntry = {
      id: id || uuidv4(),
      hostname,
      port: port || 22,
      tags: tags || [],
      mosh_allowed: mosh_allowed ?? false,
    };
    config.hosts.push(host);
    persistence.saveHosts(config);
    res.status(201).json(host);
  } catch {
    res.status(500).json({ error: 'Failed to save host' });
  }
});

router.put('/:id', (req: Request, res: Response) => {
  const { id } = req.params;
  const updates = req.body as Partial<HostEntry>;

  try {
    const config = persistence.loadHosts();
    const idx = config.hosts.findIndex(h => h.id === id);
    if (idx < 0) {
      res.status(404).json({ error: 'Host not found' });
      return;
    }
    config.hosts[idx] = { ...config.hosts[idx], ...updates, id };
    persistence.saveHosts(config);
    res.json(config.hosts[idx]);
  } catch {
    res.status(500).json({ error: 'Failed to update host' });
  }
});

router.delete('/:id', (req: Request, res: Response) => {
  const { id } = req.params;
  try {
    const config = persistence.loadHosts();
    const idx = config.hosts.findIndex(h => h.id === id);
    if (idx < 0) {
      res.status(404).json({ error: 'Host not found' });
      return;
    }
    config.hosts.splice(idx, 1);
    persistence.saveHosts(config);
    res.status(204).send();
  } catch {
    res.status(500).json({ error: 'Failed to delete host' });
  }
});

export default router;
