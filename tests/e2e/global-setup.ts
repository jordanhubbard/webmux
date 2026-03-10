import fs from 'fs';
import path from 'path';

const TEST_HOME = path.resolve(__dirname, '.test-home');
const DEFAULTS_DIR = path.resolve(__dirname, '../../webmux/config.defaults');

export default function globalSetup() {
  fs.mkdirSync(path.join(TEST_HOME, 'config'), { recursive: true });
  fs.mkdirSync(path.join(TEST_HOME, 'logs'), { recursive: true });
  fs.mkdirSync(path.join(TEST_HOME, 'data'), { recursive: true });

  for (const entry of fs.readdirSync(DEFAULTS_DIR, { withFileTypes: true })) {
    if (entry.isDirectory()) continue;
    let content = fs.readFileSync(path.join(DEFAULTS_DIR, entry.name), 'utf-8');
    if (entry.name === 'auth.yaml') {
      content = content.replace('mode: local', 'mode: none');
    }
    fs.writeFileSync(path.join(TEST_HOME, 'config', entry.name), content);
  }
}
