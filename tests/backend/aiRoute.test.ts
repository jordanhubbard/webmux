import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import express from 'express';
import request from 'supertest';

describe('POST /api/ai/explain', () => {
  let tmpDir: string;
  let originalHome: string | undefined;
  let originalGatewayUrl: string | undefined;
  let originalGatewayToken: string | undefined;
  let app: express.Express;

  beforeEach(async () => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-ai-'));
    originalHome = process.env.WEBMUX_HOME;
    originalGatewayUrl = process.env.AI_GATEWAY_URL;
    originalGatewayToken = process.env.AI_GATEWAY_TOKEN;
    process.env.WEBMUX_HOME = tmpDir;

    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });
    fs.writeFileSync(path.join(configDir, 'auth.yaml'), 'auth:\n  mode: none\n  users: []\n');

    jest.resetModules();

    const { default: aiRouter } = require('@backend/api/ai');

    app = express();
    app.use(express.json());
    app.use('/api/ai', aiRouter);
  });

  afterEach(() => {
    if (originalHome === undefined) {
      delete process.env.WEBMUX_HOME;
    } else {
      process.env.WEBMUX_HOME = originalHome;
    }
    if (originalGatewayUrl === undefined) {
      delete process.env.AI_GATEWAY_URL;
    } else {
      process.env.AI_GATEWAY_URL = originalGatewayUrl;
    }
    if (originalGatewayToken === undefined) {
      delete process.env.AI_GATEWAY_TOKEN;
    } else {
      process.env.AI_GATEWAY_TOKEN = originalGatewayToken;
    }
    jest.restoreAllMocks();
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('returns 400 when context is missing', async () => {
    const res = await request(app).post('/api/ai/explain').send({});
    expect(res.status).toBe(400);
    expect(res.body.error).toBe('context is required');
  });

  it('returns 400 when context is not a string', async () => {
    const res = await request(app).post('/api/ai/explain').send({ context: 42 });
    expect(res.status).toBe(400);
    expect(res.body.error).toBe('context is required');
  });

  it('returns AI response on success', async () => {
    const mockFetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        choices: [{ message: { content: 'This is an error in your script.' } }],
      }),
    });
    global.fetch = mockFetch as unknown as typeof fetch;

    const res = await request(app)
      .post('/api/ai/explain')
      .send({ context: 'Error: command not found: foobar' });

    expect(res.status).toBe(200);
    expect(res.body.response).toBe('This is an error in your script.');
    expect(mockFetch).toHaveBeenCalledTimes(1);

    const [url, opts] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(url).toContain('/v1/chat/completions');
    const body = JSON.parse(opts.body as string);
    expect(body.messages[0].role).toBe('system');
    expect(body.messages[1].role).toBe('user');
    expect(body.messages[1].content).toContain('Error: command not found: foobar');
  });

  it('includes question in the user message when provided', async () => {
    const mockFetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        choices: [{ message: { content: 'The command failed because...' } }],
      }),
    });
    global.fetch = mockFetch as unknown as typeof fetch;

    const res = await request(app)
      .post('/api/ai/explain')
      .send({ context: 'some output', question: 'Why did this fail?' });

    expect(res.status).toBe(200);
    expect(res.body.response).toBe('The command failed because...');

    const [, opts] = mockFetch.mock.calls[0] as [string, RequestInit];
    const body = JSON.parse(opts.body as string);
    expect(body.messages[1].content).toContain('Why did this fail?');
    expect(body.messages[1].content).toContain('some output');
  });

  it('uses Authorization header with gateway token', async () => {
    process.env.AI_GATEWAY_TOKEN = 'testtoken123';
    jest.resetModules();
    const { default: freshRouter } = require('@backend/api/ai');
    const freshApp = express();
    freshApp.use(express.json());
    freshApp.use('/api/ai', freshRouter);

    const mockFetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ choices: [{ message: { content: 'ok' } }] }),
    });
    global.fetch = mockFetch as unknown as typeof fetch;

    await request(freshApp).post('/api/ai/explain').send({ context: 'test' });

    const [, opts] = mockFetch.mock.calls[0] as [string, RequestInit];
    const headers = opts.headers as Record<string, string>;
    expect(headers['Authorization']).toBe('Bearer testtoken123');
  });

  it('returns 502 when gateway responds with error status', async () => {
    const mockFetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 500,
      text: async () => 'Internal Server Error',
    });
    global.fetch = mockFetch as unknown as typeof fetch;

    const res = await request(app)
      .post('/api/ai/explain')
      .send({ context: 'some terminal output' });

    expect(res.status).toBe(502);
    expect(res.body.error).toBe('AI gateway returned an error');
  });

  it('returns 502 when fetch throws (network error)', async () => {
    const mockFetch = jest.fn().mockRejectedValue(new Error('ECONNREFUSED'));
    global.fetch = mockFetch as unknown as typeof fetch;

    const res = await request(app)
      .post('/api/ai/explain')
      .send({ context: 'some terminal output' });

    expect(res.status).toBe(502);
    expect(res.body.error).toBe('Failed to reach AI gateway');
  });

  it('returns empty string when gateway returns no choices', async () => {
    const mockFetch = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ choices: [] }),
    });
    global.fetch = mockFetch as unknown as typeof fetch;

    const res = await request(app)
      .post('/api/ai/explain')
      .send({ context: 'some output' });

    expect(res.status).toBe(200);
    expect(res.body.response).toBe('');
  });
});
