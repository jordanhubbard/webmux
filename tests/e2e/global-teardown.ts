import fs from 'fs';
import path from 'path';

export default function globalTeardown() {
  const testHome = path.resolve(__dirname, '.test-home');
  fs.rmSync(testHome, { recursive: true, force: true });
}
