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
  const body = req.body as Partial<HostEntry>;

  if (!body.hostname) {
    res.status(400).json({ error: 'hostname is required' });
    return;
  }

  try {
    const config = persistence.loadHosts();
    const host: HostEntry = {
      id: body.id || uuidv4(),
      hostname: body.hostname,
      port: body.port || 22,
      username: body.username || '',
      transport: body.transport || 'ssh',
      key_id: body.key_id || '',
      tags: body.tags || [],
      mosh_allowed: body.mosh_allowed ?? false,
      vnc_enabled: body.vnc_enabled ?? false,
      vnc_port: body.vnc_port ?? 5900,
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
    const merged = { ...config.hosts[idx], ...updates, id };
    merged.vnc_enabled = merged.vnc_enabled ?? false;
    merged.vnc_port = merged.vnc_port ?? 5900;
    config.hosts[idx] = merged;
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
