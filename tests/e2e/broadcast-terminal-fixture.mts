import fs from 'node:fs';
const receipt = process.argv[2];
if (!receipt || !process.stdin.isTTY) throw new Error('Broadcast fixture requires an owned receipt and a real PTY');
process.stdin.setRawMode(true);
process.stdin.resume();
process.stdin.on('data', (data: Buffer) => {
  fs.appendFileSync(receipt, data);
  process.stdout.write(`\r\nINPUT:${data.toString('hex')};`);
});
process.stdout.write('BROADCAST READY');
