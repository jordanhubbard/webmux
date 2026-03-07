import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

describe('PersistenceManager', () => {
  let tmpDir: string;
  let originalRoot: string | undefined;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-test-'));
    originalRoot = process.env.WEBMUX_ROOT;
    process.env.WEBMUX_ROOT = tmpDir;
  });

  afterEach(() => {
    if (originalRoot === undefined) {
      delete process.env.WEBMUX_ROOT;
    } else {
      process.env.WEBMUX_ROOT = originalRoot;
    }
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('creates config and data directories on construction', () => {
    // Re-import to trigger constructor with new WEBMUX_ROOT
    jest.resetModules();
    const { PersistenceManager } = require('@backend/services/persistenceManager');
    const pm = new PersistenceManager();

    expect(fs.existsSync(path.join(tmpDir, 'config'))).toBe(true);
    expect(fs.existsSync(path.join(tmpDir, 'data'))).toBe(true);
    pm.close();
  });

  it('writes and reads YAML files atomically', () => {
    jest.resetModules();
    const { PersistenceManager } = require('@backend/services/persistenceManager');
    const pm = new PersistenceManager();

    // Create config dir with a yaml file
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });

    const hostConfig = {
      hosts: [
        { id: 'test01', hostname: 'test01.example.com', port: 22, tags: ['test'], mosh_allowed: false }
      ]
    };

    pm.saveHosts(hostConfig);
    const loaded = pm.loadHosts();
    expect(loaded.hosts).toHaveLength(1);
    expect(loaded.hosts[0].hostname).toBe('test01.example.com');
    pm.close();
  });

  it('appends events to JSONL file', () => {
    jest.resetModules();
    const { PersistenceManager } = require('@backend/services/persistenceManager');
    const pm = new PersistenceManager();

    pm.appendEvent({ type: 'test_event', data: 'hello' });

    const eventsDir = path.join(tmpDir, 'data', 'events');
    const files = fs.readdirSync(eventsDir);
    expect(files.length).toBeGreaterThan(0);

    const content = fs.readFileSync(path.join(eventsDir, files[0]), 'utf-8');
    const parsed = JSON.parse(content.trim());
    expect(parsed.type).toBe('test_event');
    expect(parsed.ts).toBeDefined();
    pm.close();
  });
});
