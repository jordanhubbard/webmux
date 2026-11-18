import readline from 'node:readline';
if (process.argv[2] && ['local', 'peer'].includes(process.argv[3])) {
  const fs = await import('node:fs');
  const { randomUUID } = await import('node:crypto');
  fs.appendFileSync(process.argv[2], JSON.stringify({ pid: process.pid, instance: randomUUID(), role: process.argv[3] }) + '\n', { mode: 0o600 });
}
console.log(`FEDERATION_READY:${process.pid}`);
process.stdout.on('resize', () => console.log(`FEDERATION_SIZE:${process.stdout.columns}x${process.stdout.rows}`));
readline.createInterface({ input: process.stdin }).on('line', line => {
  if (line.trim() === 'finish-provider') process.exit(0);
  console.log(`FEDERATION_REPLY:${line}`);
});
