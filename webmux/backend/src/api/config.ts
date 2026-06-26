import { Router, Request, Response } from 'express';
import { persistence } from '../services/persistenceManager';
import { requireAuth } from '../middleware/auth';
import { TransportLauncher } from '../services/transportLauncher';
import {
  appConfigWithEffectiveTerminalGridLimits,
  isTerminalGridLimitError,
  terminalGridLimitsFromApp,
} from '../services/terminalGridLimits';
import { normalizeAppConfig } from '../services/appConfig';

const router = Router();
router.use(requireAuth);

router.get('/', (_req: Request, res: Response) => {
  try {
    const app = normalizeAppConfig(appConfigWithEffectiveTerminalGridLimits(persistence.loadApp()));
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
const MUTABLE_APP_FIELDS = ['name', 'default_term', 'terminal_grid', 'transport', 'ui'];

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
    if (updates.transport?.mosh_server_path) {
      try {
        TransportLauncher.validateMoshServerPath(updates.transport.mosh_server_path);
      } catch (err) {
        res.status(400).json({ error: (err as Error).message });
        return;
      }
    }
    const merged = {
      app: {
        ...current.app,
        ...updates,
        default_term: updates.default_term
          ? { ...current.app.default_term, ...updates.default_term }
          : current.app.default_term,
        terminal_grid: updates.terminal_grid
          ? { ...current.app.terminal_grid, ...updates.terminal_grid }
          : current.app.terminal_grid,
        transport: updates.transport
          ? { ...current.app.transport, ...updates.transport }
          : current.app.transport,
        ui: updates.ui
          ? { ...current.app.ui, ...updates.ui }
          : current.app.ui,
      },
    };
    let normalized;
    try {
      terminalGridLimitsFromApp(merged);
      normalized = normalizeAppConfig(merged);
    } catch (err) {
      if (isTerminalGridLimitError(err)) {
        res.status(400).json({ error: (err as Error).message });
        return;
      }
      if ((err as Error).message === 'Invalid app.default_term.font_family') {
        res.status(400).json({ error: (err as Error).message });
        return;
      }
      throw err;
    }
    const persisted = {
      app: {
        ...merged.app,
        default_term: normalized.app.default_term,
      },
    };
    persistence.saveApp(persisted);
    res.json(normalizeAppConfig(appConfigWithEffectiveTerminalGridLimits(persisted)));
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
