import { Router } from 'express';
import { requireAuth } from '../middleware/auth';

const router = Router();

const AI_GATEWAY_URL = process.env.AI_GATEWAY_URL || 'http://localhost:18789/v1/chat/completions';
const AI_GATEWAY_TOKEN = process.env.AI_GATEWAY_TOKEN || 'clawmeh';

router.post('/explain', requireAuth, async (req, res) => {
  const { context, question } = req.body as { context?: string; question?: string };

  if (!context || typeof context !== 'string') {
    res.status(400).json({ error: 'context is required' });
    return;
  }

  const userMessage = question
    ? `Here is terminal output:\n\`\`\`\n${context}\n\`\`\`\n\n${question}`
    : `Please explain the following terminal output:\n\`\`\`\n${context}\n\`\`\``;

  try {
    const gatewayRes = await fetch(AI_GATEWAY_URL, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${AI_GATEWAY_TOKEN}`,
      },
      body: JSON.stringify({
        model: 'gpt-4o',
        messages: [
          {
            role: 'system',
            content: 'You are a helpful terminal assistant. Explain errors, suggest fixes, and answer questions about terminal output concisely.',
          },
          {
            role: 'user',
            content: userMessage,
          },
        ],
      }),
    });

    if (!gatewayRes.ok) {
      const errText = await gatewayRes.text();
      console.error('AI gateway error:', gatewayRes.status, errText);
      res.status(502).json({ error: 'AI gateway returned an error' });
      return;
    }

    const data = await gatewayRes.json() as {
      choices?: Array<{ message?: { content?: string } }>;
    };

    const response = data.choices?.[0]?.message?.content || '';
    res.json({ response });
  } catch (err) {
    console.error('AI proxy error:', err);
    res.status(502).json({ error: 'Failed to reach AI gateway' });
  }
});

export default router;
