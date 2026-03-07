import { Router, Request, Response } from 'express';
import { persistence } from '../services/persistenceManager';
import { requireAuth } from '../middleware/auth';

const router = Router();
router.use(requireAuth);

router.get('/', (_req: Request, res: Response) => {
  try {
    const app = persistence.loadApp();
    res.json(app);
  } catch {
    res.status(500).json({ error: 'Failed to load config' });
  }
});

router.put('/', (req: Request, res: Response) => {
  try {
    const current = persistence.loadApp();
    // Deep merge top-level keys
    const updated = { ...current, ...req.body };
    persistence.saveApp(updated);
    res.json(updated);
  } catch {
    res.status(500).json({ error: 'Failed to save config' });
  }
});

router.get('/layout', (_req: Request, res: Response) => {
  try {
    const layout = persistence.loadLayout();
    res.json(layout);
  } catch {
    res.status(500).json({ error: 'Failed to load layout' });
  }
});

router.put('/layout', (req: Request, res: Response) => {
  try {
    persistence.saveLayout(req.body);
    res.json(req.body);
  } catch {
    res.status(500).json({ error: 'Failed to save layout' });
  }
});

export default router;
