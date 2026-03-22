import { Router, Request, Response } from 'express';
import { requireAuth } from '../middleware/auth';
import { persistence } from '../services/persistenceManager';

const router = Router();
router.use(requireAuth);

router.post('/ask', async (req: Request, res: Response) => {
  const { prompt, context } = req.body as { prompt?: string; context?: string };

  if (!prompt || typeof prompt !== 'string' || prompt.trim().length === 0) {
    res.status(400).json({ error: 'prompt is required' });
    return;
  }

  let rccUrl = 'http://146.190.134.110:8789';
  let rccToken = '';
  try {
    const appConfig = persistence.loadApp();
    if (appConfig.ai?.rcc_url) rccUrl = appConfig.ai.rcc_url;
    if (appConfig.ai?.rcc_token) rccToken = appConfig.ai.rcc_token;
    if (appConfig.ai?.enabled === false) {
      res.status(503).json({ error: 'AI assistant is disabled in configuration' });
      return;
    }
  } catch {
    // use defaults
  }

  try {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (rccToken) headers['Authorization'] = `Bearer ${rccToken}`;

    const upstream = await fetch(`${rccUrl}/api/brain/request`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ prompt: prompt.trim(), context: context || '' }),
    });

    if (!upstream.ok) {
      const errText = await upstream.text().catch(() => '');
      res.status(502).json({ error: `AI service responded with ${upstream.status}`, details: errText });
      return;
    }

    const data = await upstream.json();
    res.json(data);
  } catch (err) {
    console.error('AI ask error:', err);
    res.status(502).json({ error: 'Failed to reach AI service' });
  }
});

export default router;
