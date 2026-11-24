// Private stdio client for the owned four-session native fixture. No listener,
// credentials, config rewrites or request replay; EOF closes the native channel.
import readline from 'node:readline';
import { authorizeSocket, connect } from './probe-codex-task-tools.mts';

const socket = process.argv[2];
if (!socket || process.argv.length !== 3) throw new Error('One private Unix socket required');
const channel = await connect(await authorizeSocket(socket));
try {
  for await (const line of readline.createInterface({ input: process.stdin, crlfDelay: Infinity })) {
    const request = JSON.parse(line) as { id?: unknown; method?: unknown; params?: unknown };
    if (!Number.isSafeInteger(request.id) || !['thread/read', 'thread/turns/list', 'turn/start', 'mcpServerStatus/list', 'mcpServer/tool/call'].includes(String(request.method)) || !request.params || typeof request.params !== 'object' || Array.isArray(request.params)) throw new Error('Invalid fixture request');
    try {
      const result = await channel.call(request.method as Parameters<typeof channel.call>[0], request.params as Record<string, unknown>);
      process.stdout.write(`${JSON.stringify({ id: request.id, result })}\n`);
    } catch {
      // Transport errors can contain capabilities. The owner records the fixed
      // failure and retains its private daemon log rather than forwarding text.
      process.stdout.write(`${JSON.stringify({ id: request.id, error: 'native-rpc-failed' })}\n`);
    }
  }
} finally { channel.close(); }
