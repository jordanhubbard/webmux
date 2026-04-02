/**
 * /api/ai — AI assistant for webmux terminal sessions.
 *
 * POST /api/ai/chat
 *   Body: { message: string, context?: string, sessionId?: string, history?: Message[] }
 *   Returns: { reply: string, model: string, ts: number }
 *
 * The request is forwarded to the RCC brain API (WEBMUX_RCC_URL + WEBMUX_RCC_TOKEN).
 * If RCC is not configured, falls back to the configured OPENAI_API_KEY or NVIDIA_API_KEY.
 *
 * Terminal context (last N lines of terminal output) is automatically prepended to the
 * system message so the assistant can explain errors or suggest next commands.
 */

import { Router, Request, Response } from 'express';

const router = Router();

interface Message {
  role: 'system' | 'user' | 'assistant';
  content: string;
}

interface ChatRequest {
  message:   string;
  context?:  string;  // last N lines of terminal output
  sessionId?: string;
  history?:  Message[];
}

const SYSTEM_PROMPT = `You are a terminal assistant embedded in webmux, a browser-based terminal multiplexer.
You help users understand command output, diagnose errors, and suggest next steps.
When you see terminal output in <terminal_context> tags, use it to give specific, actionable advice.
Keep responses concise and focused. Prefer shell commands over lengthy explanations.
Format commands in backticks. Do not repeat the terminal context back to the user.`;

async function callRCCBrain(messages: Message[]): Promise<string> {
  const rccUrl   = process.env.WEBMUX_RCC_URL || process.env.LOOM_RCC_BRAIN_URL || '';
  const rccToken = process.env.WEBMUX_RCC_TOKEN || process.env.LOOM_RCC_AGENT_TOKEN || '';

  if (!rccUrl) throw new Error('RCC not configured');

  const resp = await fetch(`${rccUrl.replace(/\/$/, '')}/api/brain/request`, {
    method:  'POST',
    headers: {
      'Content-Type':  'application/json',
      'Authorization': `Bearer ${rccToken}`,
    },
    body: JSON.stringify({ messages, maxTokens: 512, priority: 'normal',
                           metadata: { source: 'webmux-ai' } }),
    signal: AbortSignal.timeout(15_000),
  });

  if (!resp.ok) throw new Error(`RCC HTTP ${resp.status}`);
  const data = await resp.json() as { status: string; result: string; error?: string };
  if (data.status !== 'completed') throw new Error(`Brain status: ${data.status}`);
  return data.result;
}

async function callDirectLLM(messages: Message[]): Promise<string> {
  const nvKey  = process.env.NVIDIA_API_KEY || '';
  const oaKey  = process.env.OPENAI_API_KEY || '';

  const apiKey = nvKey || oaKey;
  if (!apiKey) throw new Error('No LLM API key configured');

  const baseUrl = nvKey
    ? 'https://integrate.api.nvidia.com/v1'
    : 'https://api.openai.com/v1';
  const model = nvKey
    ? (process.env.WEBMUX_MODEL || 'meta/llama-3.1-70b-instruct')
    : (process.env.WEBMUX_MODEL || 'gpt-4o-mini');

  const resp = await fetch(`${baseUrl}/chat/completions`, {
    method:  'POST',
    headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${apiKey}` },
    body:    JSON.stringify({ model, messages, max_tokens: 512, temperature: 0.3 }),
    signal:  AbortSignal.timeout(20_000),
  });

  if (!resp.ok) throw new Error(`LLM API HTTP ${resp.status}`);
  const data = await resp.json() as {
    choices: Array<{ message: { content: string } }>;
    model: string;
  };
  return data.choices[0]?.message?.content?.trim() || '';
}

router.post('/chat', async (req: Request, res: Response) => {
  const { message, context, history = [] } = req.body as ChatRequest;

  if (!message || typeof message !== 'string' || message.trim().length === 0) {
    return res.status(400).json({ error: 'message required' });
  }

  // Build message array
  const messages: Message[] = [{ role: 'system', content: SYSTEM_PROMPT }];

  // Inject terminal context if provided
  const userContent = context && context.trim()
    ? `<terminal_context>\n${context.trim().slice(-3000)}\n</terminal_context>\n\n${message.trim()}`
    : message.trim();

  // Append conversation history (last 10 turns to keep context manageable)
  const recentHistory = history.slice(-10);
  messages.push(...recentHistory.filter(m => m.role !== 'system'));
  messages.push({ role: 'user', content: userContent });

  let reply = '';
  let model = 'unknown';
  let source = 'rcc';

  try {
    reply = await callRCCBrain(messages);
    model = 'rcc-brain';
  } catch (rccErr) {
    // Fallback to direct LLM
    source = 'direct';
    try {
      reply = await callDirectLLM(messages);
      model = process.env.WEBMUX_MODEL || 'llm';
    } catch (llmErr) {
      const errMsg = llmErr instanceof Error ? llmErr.message : String(llmErr);
      return res.status(503).json({
        error: 'AI assistant unavailable',
        detail: errMsg,
        hint: 'Set WEBMUX_RCC_URL+WEBMUX_RCC_TOKEN or NVIDIA_API_KEY/OPENAI_API_KEY',
      });
    }
  }

  return res.json({ reply, model, source, ts: Date.now() });
});

// GET /api/ai/status — check if AI is configured
router.get('/status', (_req: Request, res: Response) => {
  const hasRCC    = !!(process.env.WEBMUX_RCC_URL || process.env.LOOM_RCC_BRAIN_URL);
  const hasNVIDIA = !!process.env.NVIDIA_API_KEY;
  const hasOpenAI = !!process.env.OPENAI_API_KEY;
  const available = hasRCC || hasNVIDIA || hasOpenAI;
  res.json({
    available,
    providers: { rcc: hasRCC, nvidia: hasNVIDIA, openai: hasOpenAI },
    model: process.env.WEBMUX_MODEL || (hasNVIDIA ? 'meta/llama-3.1-70b-instruct' : 'gpt-4o-mini'),
  });
});

export default router;
