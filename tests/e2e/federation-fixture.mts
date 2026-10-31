import readline from 'node:readline';
console.log(`FEDERATION_READY:${process.pid}`);
readline.createInterface({ input: process.stdin }).on('line', line => console.log(`FEDERATION_REPLY:${line}`));
