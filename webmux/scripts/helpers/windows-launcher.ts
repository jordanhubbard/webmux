import path from 'node:path';

if (process.versions.node.split('.')[0] !== '24') {
  console.error('This WebMux bundle requires Node.js 24 on PATH.');
  process.exit(1);
}

const root = path.resolve(__dirname, '..');
process.env.WEBMUX_ROOT = root;
require(path.join(root, 'backend/dist/index.js'));
