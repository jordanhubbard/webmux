import { Router, Request, Response } from 'express';
import { persistence } from '../services/persistenceManager';
import { requireAuth } from '../middleware/auth';

const router = Router();
router.use(requireAuth);

router.get('/', (_req: Request, res: Response) => {
  try {
    const app = persistence.loadApp();
    const execCommand = process.env.WEBMUX_EXEC_COMMAND;
    if (execCommand) {
      app.app.exec_command = execCommand;
    }
    res.json(app);
  } catch {
    res.status(500).json({ error: 'Failed to load config' });
  }
});

// Only allow updating safe fields — not listen_host, ports, or secure_mode at runtime
const MUTABLE_APP_FIELDS = ['name', 'default_term', 'transport'];

router.put('/', (req: Request, res: Response) => {
  try {
    const current = persistence.loadApp();
    const updates = req.body?.app;
    if (!updates || typeof updates !== 'object') {
      res.status(400).json({ error: 'Request body must contain an app object' });
      return;
    }
    for (const key of Object.keys(updates)) {
      if (!MUTABLE_APP_FIELDS.includes(key)) {
        res.status(400).json({ error: `Field '${key}' cannot be changed at runtime` });
        return;
      }
    }
    const merged = { app: { ...current.app, ...updates } };
    persistence.saveApp(merged);
    res.json(merged);
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
    if (!req.body?.layout) {
      res.status(400).json({ error: 'Request body must contain a layout object' });
      return;
    }
    persistence.saveLayout(req.body);
    res.json(req.body);
  } catch {
    res.status(500).json({ error: 'Failed to save layout' });
  }
});

export default router;
