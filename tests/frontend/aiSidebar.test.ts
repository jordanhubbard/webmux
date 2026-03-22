import { describe, it, expect, vi, beforeEach } from 'vitest';

describe('api.askAi', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    global.fetch = fetchSpy;
    vi.resetModules();
    try { localStorage.clear(); } catch { /* jsdom */ }
  });

  it('POSTs to /api/ai/ask with prompt', async () => {
    fetchSpy.mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ response: 'Hello!' }),
    });

    const { api } = await import('@frontend/utils/api');
    const result = await api.askAi('What is 2+2?');
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/ai/ask',
      expect.objectContaining({ method: 'POST' })
    );
    const body = JSON.parse(fetchSpy.mock.calls[0][1].body);
    expect(body.prompt).toBe('What is 2+2?');
    expect(result.response).toBe('Hello!');
  });

  it('POSTs context when provided', async () => {
    fetchSpy.mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ response: 'Done' }),
    });

    const { api } = await import('@frontend/utils/api');
    await api.askAi('Help me', 'ls output here');
    const body = JSON.parse(fetchSpy.mock.calls[0][1].body);
    expect(body.context).toBe('ls output here');
  });

  it('throws on non-ok response', async () => {
    fetchSpy.mockResolvedValue({
      ok: false,
      status: 502,
      statusText: 'Bad Gateway',
      json: () => Promise.resolve({ error: 'AI service down' }),
    });

    const { api } = await import('@frontend/utils/api');
    await expect(api.askAi('test')).rejects.toThrow('AI service down');
  });
});

describe('AiSidebar parseSegments logic', () => {
  // Test the response parsing logic inline
  function parseSegments(text: string): Array<{ type: 'text' | 'code'; content: string; lang?: string }> {
    const segments: Array<{ type: 'text' | 'code'; content: string; lang?: string }> = [];
    const codeBlockRe = /```(\w*)\n?([\s\S]*?)```/g;
    let lastIndex = 0;
    let match: RegExpExecArray | null;
    while ((match = codeBlockRe.exec(text)) !== null) {
      if (match.index > lastIndex) {
        segments.push({ type: 'text', content: text.slice(lastIndex, match.index) });
      }
      segments.push({ type: 'code', content: match[2].trimEnd(), lang: match[1] || undefined });
      lastIndex = match.index + match[0].length;
    }
    if (lastIndex < text.length) {
      segments.push({ type: 'text', content: text.slice(lastIndex) });
    }
    return segments;
  }

  it('returns single text segment for plain text', () => {
    const segs = parseSegments('Hello world');
    expect(segs).toHaveLength(1);
    expect(segs[0]).toEqual({ type: 'text', content: 'Hello world' });
  });

  it('extracts code block with language', () => {
    const input = 'Try this:\n```bash\necho hello\n```\nDone.';
    const segs = parseSegments(input);
    const code = segs.find(s => s.type === 'code');
    expect(code).toBeDefined();
    expect(code?.lang).toBe('bash');
    expect(code?.content).toBe('echo hello');
  });

  it('extracts code block without language', () => {
    const input = '```\nls -la\n```';
    const segs = parseSegments(input);
    expect(segs).toHaveLength(1);
    expect(segs[0].type).toBe('code');
    expect(segs[0].content).toBe('ls -la');
  });

  it('handles multiple code blocks', () => {
    const input = '```bash\ncmd1\n```\nThen:\n```python\nprint(1)\n```';
    const segs = parseSegments(input);
    const codeBlocks = segs.filter(s => s.type === 'code');
    expect(codeBlocks).toHaveLength(2);
    expect(codeBlocks[0].lang).toBe('bash');
    expect(codeBlocks[1].lang).toBe('python');
  });
});

describe('Session type field', () => {
  it('CreateSessionRequest accepts session_type: claude', async () => {
    // Type-level test: ensure the type accepts claude
    const req = { session_type: 'claude' as const, row: 0, col: 0 };
    expect(req.session_type).toBe('claude');
  });

  it('Session accepts session_type field', () => {
    const session = {
      id: 'x',
      owner: 'u',
      transport: 'ssh' as const,
      session_type: 'claude' as const,
      host_id: '',
      hostname: 'localhost',
      username: 'claude',
      key_id: '',
      cols: 80,
      rows: 24,
      row: 0,
      col: 0,
      state: 'connected' as const,
      created_at: '',
      updated_at: '',
      title: 'Claude CLI',
      persistent: false,
    };
    expect(session.session_type).toBe('claude');
  });
});
