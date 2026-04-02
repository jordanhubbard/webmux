/**
 * /api/sessions/templates — pre-configured session templates
 *
 * GET /api/sessions/templates       — list all templates
 * GET /api/sessions/templates/:id   — get a specific template
 *
 * Templates define a curated session setup: an initial command to run after
 * SSH connection, a step-by-step setup guide, and UI metadata (icon, name).
 *
 * The "claude-cli" template guides the user through:
 *   1. Installing Claude CLI (npm install -g @anthropic-ai/claude-cli or Homebrew)
 *   2. Authenticating via SSO (claude auth login → browser OAuth flow)
 *   3. Starting an interactive Claude session
 *
 * The initial_cmd from a template is passed to CreateSessionRequest.initial_cmd
 * and injected into the PTY after the SSH connection is established.
 */

import { Router, Request, Response } from 'express';
import type { SessionTemplate } from '../types';

const router = Router();

// ── Built-in templates ───────────────────────────────────────────────────────
const TEMPLATES: SessionTemplate[] = [
  {
    id:   'claude-cli',
    name: 'Claude CLI',
    icon: '🤖',
    description: 'Start a Claude AI conversation in your terminal. Installs Claude CLI and launches an interactive session.',
    initialCmd: 'claude',
    setupSteps: [
      'Ensure Node.js 18+ is installed: node --version',
      'Install Claude CLI: npm install -g @anthropic-ai/claude-cli',
      'Authenticate via SSO: claude auth login  (opens browser for OAuth)',
      'Start a session: claude  (or use the template button)',
    ],
  },
  {
    id:   'htop',
    name: 'System Monitor',
    icon: '📊',
    description: 'Launch htop for real-time process monitoring.',
    initialCmd: 'htop',
    setupSteps: [
      'Install htop if needed: sudo apt-get install htop',
      'Press q or F10 to quit.',
    ],
  },
  {
    id:   'python-repl',
    name: 'Python REPL',
    icon: '🐍',
    description: 'Start an interactive Python 3 session.',
    initialCmd: 'python3',
    setupSteps: [
      'Ensure Python 3 is installed: python3 --version',
      'Type exit() or Ctrl+D to quit.',
    ],
  },
  {
    id:   'nano-repl',
    name: 'Nano Lang REPL',
    icon: '🔷',
    description: 'Start the nanolang interactive REPL (requires nanolang installed).',
    initialCmd: 'nano --repl',
    setupSteps: [
      'Install nanolang from https://github.com/jordanhubbard/nanolang',
      'Build: make -f Makefile.gnu all',
      'Run: nano --repl',
      'Type :help for REPL commands. :load to load .nano files. :reload for hot-reload.',
    ],
  },
  {
    id:   'ssh-agent',
    name: 'SSH Agent',
    icon: '🔑',
    description: 'Start ssh-agent and add your default key for passwordless auth.',
    initialCmd: 'eval $(ssh-agent -s) && ssh-add ~/.ssh/id_rsa && echo "SSH agent ready"',
    setupSteps: [
      'Ensure ssh-agent is available: which ssh-agent',
      'Add your key: ssh-add ~/.ssh/id_rsa',
      'Verify: ssh-add -l',
    ],
  },
];

// GET /api/sessions/templates
router.get('/', (_req: Request, res: Response) => {
  res.json({ templates: TEMPLATES, count: TEMPLATES.length });
});

// GET /api/sessions/templates/:id
router.get('/:id', (req: Request, res: Response) => {
  const template = TEMPLATES.find(t => t.id === req.params.id);
  if (!template) {
    return res.status(404).json({ error: `Template '${req.params.id}' not found` });
  }
  return res.json(template);
});

export { TEMPLATES };
export default router;
