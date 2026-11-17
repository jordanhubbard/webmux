import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

test('optional RDP proxy check distinguishes missing, legacy and qualified daemons', { skip: process.platform === 'win32' }, () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-guacd-check-'));
  try {
    const awk = spawnSync('/bin/sh', ['-c', 'command -v awk'], { encoding: 'utf8' }).stdout.trim();
    fs.symlinkSync(awk, path.join(directory, 'awk'));
    const script = path.join(import.meta.dirname, 'check-guacd.sh');
    const check = () => {
      const result = spawnSync('/bin/sh', [script], { env: { ...process.env, PATH: directory }, encoding: 'utf8', timeout: 5000 });
      assert.ifError(result.error);
      assert.equal(result.status, 0, 'optional RDP check blocked terminal startup');
      return result.stdout;
    };
    assert.match(check(), /guacd not found/);
    for (const version of ['1.3.0', '1.5.5']) {
      fs.writeFileSync(path.join(directory, 'guacd'), `#!/bin/sh\necho 'Guacamole proxy daemon (guacd) version ${version}'\n`, { mode: 0o700 });
      assert.match(check(), /requires guacd 1\.6\.0/);
      assert.doesNotMatch(check(), /✓/);
    }
    fs.writeFileSync(path.join(directory, 'guacd'), "#!/bin/sh\necho 'Guacamole proxy daemon (guacd) version 1.6.0'\n", { mode: 0o700 });
    assert.match(check(), /✓ guacd 1\.6\.0 found/);
    fs.writeFileSync(path.join(directory, 'guacd'), '#!/bin/sh\nexit 1\n', { mode: 0o700 });
    assert.match(check(), /version unknown/);
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});
