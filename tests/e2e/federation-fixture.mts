import readline from 'node:readline';
console.log(`FEDERATION_READY:${process.pid}`);
process.stdout.on('resize', () => console.log(`FEDERATION_SIZE:${process.stdout.columns}x${process.stdout.rows}`));
readline.createInterface({ input: process.stdin }).on('line', line => {
  if (line.trim() === 'finish-provider') process.exit(0);
  console.log(`FEDERATION_REPLY:${line}`);
});
