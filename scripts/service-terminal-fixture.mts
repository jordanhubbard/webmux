// A real PTY child used only by service lifecycle tests.
import assert from 'node:assert/strict';

const marker = process.argv[2];
assert(marker && /^[a-f0-9]{16}$/.test(marker));
assert(process.stdin.isTTY && process.stdout.isTTY, 'Fixture requires a real PTY');
process.stdin.setRawMode(true);
process.stdin.resume();
process.stdout.write(`child:${process.pid}:ready\r\n`);
for (let index = 0; index < 128; index++) process.stdout.write(`${marker}:${index}:done\r\n`);
