import { Router, Request, Response } from 'express';
import * as fs from 'fs';
import * as path from 'path';
import { persistence } from '../services/persistenceManager';
import { requireAuth } from '../middleware/auth';
import { TransportLauncher } from '../services/transportLauncher';
import {
  appConfigWithEffectiveTerminalGridLimits,
  isTerminalGridLimitError,
  terminalGridLimitsFromApp,
} from '../services/terminalGridLimits';
import { FONT_FACE_CONFIG_ERROR, normalizeAppConfig } from '../services/appConfig';
import type { AppConfig } from '../types';

const router = Router();
router.use(requireAuth);

const FONT_CONTENT_TYPES: Record<string, string> = {
  '.otf': 'font/otf',
  '.ttf': 'font/ttf',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
};

function appConfigDir(): string {
  const appConfigFile = persistence.configPath('app.yaml');
  try {
    return path.dirname(fs.realpathSync(appConfigFile));
  } catch {
    return path.dirname(appConfigFile);
  }
}

function withFontFaceUrls(config: AppConfig): AppConfig {
  return {
    app: {
      ...config.app,
      font_faces: (config.app.font_faces ?? []).map((face, index) => ({
        ...face,
        url: `/api/config/fonts/${index}`,
      })),
    },
  };
}

function configuredFontFile(index: number): { file: string; contentType: string } | null {
  const app = normalizeAppConfig(appConfigWithEffectiveTerminalGridLimits(persistence.loadApp()));
  const face = app.app.font_faces?.[index];
  if (!face) return null;
  const file = path.resolve(appConfigDir(), face.source);
  const contentType = FONT_CONTENT_TYPES[path.extname(file).toLowerCase()];
  if (!contentType) return null;
  return { file, contentType };
}

router.get('/', (_req: Request, res: Response) => {
  try {
    const app = withFontFaceUrls(normalizeAppConfig(appConfigWithEffectiveTerminalGridLimits(persistence.loadApp())));
    const execCommand = process.env.WEBMUX_EXEC_COMMAND;
    if (execCommand) {
      app.app.exec_command = execCommand;
    }
    res.json(app);
  } catch {
    res.status(500).json({ error: 'Failed to load config' });
  }
});

router.get('/fonts/:index', (req: Request, res: Response) => {
  const index = Number(req.params.index);
  if (!Number.isInteger(index) || index < 0) {
    res.status(404).json({ error: 'Font not found' });
    return;
  }
  try {
    const fontFile = configuredFontFile(index);
    if (!fontFile || !fs.existsSync(fontFile.file) || !fs.statSync(fontFile.file).isFile()) {
      res.status(404).json({ error: 'Font not found' });
      return;
    }
    res.type(fontFile.contentType);
    res.setHeader('Cache-Control', 'private, max-age=3600');
    res.sendFile(fontFile.file);
  } catch {
    res.status(404).json({ error: 'Font not found' });
  }
});

// Only allow updating safe fields — not listen_host, ports, or secure_mode at runtime
const MUTABLE_APP_FIELDS = ['name', 'default_term', 'font_faces', 'terminal_grid', 'transport', 'ui'];

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
        font_faces: updates.font_faces !== undefined ? updates.font_faces : current.app.font_faces,
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
      if ((err as Error).message === FONT_FACE_CONFIG_ERROR) {
        res.status(400).json({ error: (err as Error).message });
        return;
      }
      throw err;
    }
    const persisted: AppConfig = {
      app: {
        ...merged.app,
        default_term: normalized.app.default_term,
        font_faces: normalized.app.font_faces,
      },
    };
    persistence.saveApp(persisted);
    res.json(withFontFaceUrls(normalizeAppConfig(appConfigWithEffectiveTerminalGridLimits(persisted))));
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
