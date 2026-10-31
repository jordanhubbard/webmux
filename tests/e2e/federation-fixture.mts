import readline from 'node:readline';
console.log(`FEDERATION_READY:${process.pid}`);
readline.createInterface({ input: process.stdin }).on('line', line => {
  if (line.trim() === 'finish-provider') process.exit(0);
  console.log(`FEDERATION_REPLY:${line}`);
});
