const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawnSync } = require('child_process');

const frontendDir = path.resolve(__dirname, '..', 'frontend');
const vitestCli = require.resolve('vitest/vitest.mjs', { paths: [frontendDir] });
const storageFile = path.join(os.tmpdir(), `webmux-vitest-localstorage-${process.pid}.json`);
const nodeMajor = Number(process.versions.node.split('.')[0]);
const childEnv = { ...process.env };
if (nodeMajor >= 22) {
  const storageOption = `--localstorage-file=${JSON.stringify(storageFile)}`;
  childEnv.NODE_OPTIONS = [childEnv.NODE_OPTIONS, storageOption].filter(Boolean).join(' ');
}

const result = spawnSync(
  process.execPath,
  [vitestCli, 'run', '--no-file-parallelism', ...process.argv.slice(2)],
  { cwd: frontendDir, env: childEnv, stdio: 'inherit' },
);

try {
  fs.unlinkSync(storageFile);
} catch {
  // The Node version may not have created a local-storage backing file.
}

if (result.error) throw result.error;
process.exitCode = result.status ?? 1;
