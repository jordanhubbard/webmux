import { Router, Request, Response, NextFunction } from 'express';
import { requireAuth, AuthPayload } from '../middleware/auth';
import { agentService } from '../services/agentService';
import { sessionBroker } from '../services/sessionBroker';
import { getAgentAccess } from '../services/agentAccess';

const router = Router();

function getOwner(req: Request): string {
  return (req as Request & { user?: AuthPayload }).user?.sub || 'anonymous';
}

function parseTermSize(body: { cols?: unknown; rows?: unknown }) {
  const cols = typeof body.cols === 'number' && Number.isFinite(body.cols) ? body.cols : 120;
  const rows = typeof body.rows === 'number' && Number.isFinite(body.rows) ? body.rows : 40;
  return {
    cols: Math.max(40, Math.min(240, Math.floor(cols))),
    rows: Math.max(10, Math.min(80, Math.floor(rows))),
  };
}

async function requireAgentAccess(_req: Request, res: Response, next: NextFunction): Promise<void> {
  try {
    const access = getAgentAccess();
    if (!access.allowed) {
      if (access.status !== 500) {
        await sessionBroker.deleteAgentSessions(access.error);
      }
      res.status(access.status).json({ error: access.error });
      return;
    }

    await sessionBroker.enforceAgentAccessPolicy();
    next();
  } catch (err) {
    console.error('Agent access cleanup failed:', err);
    res.status(500).json({ error: 'Failed to enforce agent access policy' });
  }
}

function sendInvalidAgent(res: Response): void {
  res.status(404).json({ error: 'Agent definition not found' });
}

router.use(requireAuth);

router.get('/config', (_req: Request, res: Response) => {
  try {
    res.json(agentService.getRuntimeConfig());
  } catch (err) {
    res.status(500).json({ error: (err as Error).message });
  }
});

router.get('/sessions', requireAgentAccess, async (_req: Request, res: Response) => {
  try {
    res.json(await agentService.listAllSessions());
  } catch (err) {
    console.error('Agent list error:', err);
    res.status(503).json({ error: 'Failed to list agent sessions' });
  }
});

router.get('/:agentId/sessions', requireAgentAccess, async (req: Request, res: Response) => {
  const config = agentService.getConfig(req.params.agentId);
  if (!config) {
    sendInvalidAgent(res);
    return;
  }

  try {
    res.json(await agentService.listSessions(config.id));
  } catch (err) {
    console.error(`${config.label} list error:`, err);
    res.status(503).json({ error: `Failed to list ${config.label} sessions` });
  }
});

router.post('/:agentId/attach', requireAgentAccess, async (req: Request, res: Response) => {
  const config = agentService.getConfig(req.params.agentId);
  if (!config) {
    sendInvalidAgent(res);
    return;
  }

  try {
    const { name } = req.body as { name?: string };
    if (!name || typeof name !== 'string') {
      res.status(400).json({ error: 'name is required' });
      return;
    }

    if (!(await agentService.hasSession(config.id, name))) {
      res.status(404).json({ error: `${config.label} session not found` });
      return;
    }

    const { cols, rows } = parseTermSize(req.body as { cols?: unknown; rows?: unknown });
    const result = await sessionBroker.ensureAgentAttach(
      getOwner(req),
      config.id,
      config.workspace,
      name,
      cols,
      rows,
      agentService.buildAttachExecArgv(config.id, name),
    );
    res.status(result.created ? 201 : 200).json(result.session);
  } catch (err) {
    console.error(`${config.label} attach error:`, err);
    res.status(500).json({ error: `Failed to attach ${config.label} session` });
  }
});

router.post('/:agentId/scratch', requireAgentAccess, async (req: Request, res: Response) => {
  const config = agentService.getConfig(req.params.agentId);
  if (!config) {
    sendInvalidAgent(res);
    return;
  }

  try {
    const { selectedName } = req.body as { selectedName?: string };
    if (selectedName !== undefined && typeof selectedName !== 'string') {
      res.status(400).json({ error: 'selectedName must be a string' });
      return;
    }
    const { cols, rows } = parseTermSize(req.body as { cols?: unknown; rows?: unknown });
    let cwd: string | undefined;
    if (selectedName) {
      if (!(await agentService.hasSession(config.id, selectedName))) {
        res.status(404).json({ error: `${config.label} session not found` });
        return;
      }
      cwd = await agentService.getPaneCurrentPath(config.id, selectedName);
    }
    const result = await sessionBroker.ensureAgentScratch(getOwner(req), config.id, config.workspace, cols, rows, cwd);
    res.status(result.created ? 201 : 200).json(result.session);
  } catch (err) {
    console.error(`${config.label} scratch error:`, err);
    res.status(500).json({ error: `Failed to create ${config.label} scratch shell` });
  }
});

export default router;
