// Resolve source Make listeners without writing configuration or printing it.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
const require = createRequire(new URL('../webmux/package.json', import.meta.url));
const yaml = require('js-yaml') as { load(text: string): unknown };
const invalid = () => new Error('Invalid runtime port configuration');
const mapping = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);

export function runtimePorts(home: string, defaultsFile: string, overrides: Partial<Pick<NodeJS.ProcessEnv, 'HTTP_PORT' | 'HTTPS_PORT'>> = {}) {
  if (!path.isAbsolute(home)) throw invalid();
  const configured = path.join(home, 'config/app.yaml');
  let filename = configured;
  try { fs.lstatSync(configured); }
  catch (error) {
    if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw invalid();
    filename = defaultsFile;
  }
  // Follow operator-owned file symlinks, but never silently replace a dangling
  // link with defaults or read special files which could block startup.
  if (!fs.statSync(filename).isFile()) throw invalid();
  const document = yaml.load(fs.readFileSync(filename, 'utf8'));
  if (document === undefined || (document !== null && !mapping(document))) throw invalid();
  const app = document === null ? null : (document as Record<string, unknown>).app;
  if (app !== undefined && app !== null && !mapping(app)) throw invalid();
  const saved = (app ?? {}) as Record<string, unknown>;
  const selected = (field: string, variable: 'HTTP_PORT' | 'HTTPS_PORT') => {
    // An existing partial file is not merged with defaults by the Go store.
    const value = saved[field] ?? 0;
    if (typeof value !== 'number' || !Number.isInteger(value)) throw invalid();
    const raw = overrides[variable];
    if (raw !== undefined && raw !== '' && !/^[+-]?\d+$/.test(raw)) throw invalid();
    const result = raw === undefined || raw === '' ? value : Number(raw);
    if (!Number.isInteger(result) || result < 0 || result > 65535) throw invalid();
    return result;
  };
  return { http: selected('http_port', 'HTTP_PORT'), https: selected('https_port', 'HTTPS_PORT') };
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  try {
    const home = process.env.WEBMUX_HOME ?? path.join(os.homedir(), '.config/webmux');
    const root = process.env.WEBMUX_ROOT ?? path.resolve(import.meta.dirname, '../webmux');
    const ports = runtimePorts(home, path.join(root, 'config.defaults/app.yaml'), process.env);
    console.log(`${ports.http} ${ports.https}`);
  } catch {
    console.error('WebMux: Cannot resolve listener ports; check app.yaml and HTTP_PORT/HTTPS_PORT overrides.');
    process.exitCode = 1;
  }
}
