// A real terminal child used by the browser keyboard tests.
if (!process.stdin.isTTY) throw new Error('Browser terminal fixture requires a PTY');
process.stdin.setRawMode(true);
process.stdin.resume();
process.stdin.on('data', (data: Buffer) => process.stdout.write(`KEY:${data.toString('hex')};`));
process.stdout.write('READY');
