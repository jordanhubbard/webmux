// Deterministic application output through a real PTY, not a mocked WebSocket.
if (!process.stdin.isTTY) throw new Error('Visual terminal fixture requires a PTY');
process.stdin.setRawMode(true);
process.stdin.resume();
process.stdout.write([
  '\x1b[?25l\x1b[2J\x1b[HWebMux terminal fixture',
  '\x1b[31mred\x1b[0m \x1b[32mgreen\x1b[0m \x1b[1mbold\x1b[0m',
  'Unicode: λ 日本語',
  'needle first',
  'needle second',
  'fixture ready',
  '',
].join('\r\n'));
let pending = '';
process.stdin.on('data', (data: Buffer) => {
  pending += data.toString('utf8');
  let end: number;
  while ((end = pending.indexOf('\r')) >= 0) {
    const line = pending.slice(0, end);
    pending = pending.slice(end + 1);
    if (line === 'exit') {
      process.stdout.write('fixture stopped\r\n', () => process.exit(0));
      return;
    }
    process.stdout.write(`input: ${line}\r\n`);
  }
});
