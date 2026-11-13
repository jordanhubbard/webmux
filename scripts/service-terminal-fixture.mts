// A real PTY child used only by service lifecycle tests.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const marker = process.argv[2];
assert(marker && /^[a-f0-9]{16}$/.test(marker));
assert(process.stdin.isTTY && process.stdout.isTTY, 'Fixture requires a real PTY');
const [expectedProfile, expectedUser, installerProfile] = process.argv.slice(3);
if (expectedProfile !== undefined) {
  try {
    assert(expectedUser && installerProfile);
    const samePath = (a: string | undefined, b: string) => a !== undefined && path.resolve(a).toLowerCase() === path.resolve(b).toLowerCase();
    assert(samePath(process.env.USERPROFILE, expectedProfile), 'Service USERPROFILE differs from its registered account profile');
    assert(samePath(process.env.HOME, expectedProfile), 'Service HOME differs from its registered account profile');
    assert(samePath(os.homedir(), expectedProfile), 'Native home lookup differs from the registered account profile');
    assert.equal(os.userInfo().username.toLowerCase(), expectedUser.toLowerCase(), 'Service PTY uses another account');
    assert(!process.env.PATH?.toLowerCase().includes(installerProfile.toLowerCase()), 'Service PATH contains the installer-only profile');
    const proof = path.join(expectedProfile, `.webmux-service-profile-${marker}`);
    let created = false;
    try {
      fs.writeFileSync(proof, marker, { flag: 'wx' });
      created = true;
      assert.equal(fs.readFileSync(proof, 'utf8'), marker);
    } finally { if (created) fs.rmSync(proof, { force: true }); }
    process.stdout.write(`profile:${marker}:ok\r\n`);
  } catch (error) {
    process.stdout.write(`profile:${marker}:failed\r\n`);
    throw error;
  }
}
process.stdin.setRawMode(true);
process.stdin.resume();
process.stdout.write(`child:${process.pid}:ready\r\n`);
for (let index = 0; index < 128; index++) process.stdout.write(`${marker}:${index}:done\r\n`);
