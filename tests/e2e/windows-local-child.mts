import fs from 'node:fs';
const [receipt, nonce] = process.argv.slice(2);
if (!receipt || !/^WIN_LOCAL_[a-f0-9]{32}$/.test(nonce) || !process.stdin.isTTY) throw Error('Expected an owned native terminal fixture');
fs.writeFileSync(receipt, JSON.stringify({ pid: process.pid, nonce, expiresAfter: Date.now() + 60_000 }), { flag: 'wx', mode: 0o600 });
console.log(nonce);
// Stop must end this child before this independent fixture expiry.
setTimeout(() => process.exit(0), 60_000);
