import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { realpathSync } from 'node:fs';
import { dirname, resolve } from 'node:path';

/** Use compiler CLIs for resolved options and the actual checked source graph. */
export function checkedProjectSources(compiler: string, project: string): Set<string> {
  const file = resolve(project);
  const output = execFileSync(process.execPath, [compiler, '--showConfig', '--project', file],
    { encoding: 'utf8', timeout: 60_000, maxBuffer: 16 << 20 });
  const value: unknown = JSON.parse(output);
  assert(value && typeof value === 'object' && !Array.isArray(value), `${file}: invalid compiler config`);
  const config = value as Record<string, unknown>;
  assert(config.compilerOptions && typeof config.compilerOptions === 'object' && !Array.isArray(config.compilerOptions), `${file}: missing compiler options`);
  assert.equal((config.compilerOptions as Record<string, unknown>).strict, true, `${file} must enable strict checking`);
  // --showConfig may omit files already matched by include patterns. The
  // compiler's actual file graph also includes imports it will type-check.
  const inventory = execFileSync(process.execPath, [compiler, '--listFilesOnly', '--project', file],
    { encoding: 'utf8', timeout: 60_000, maxBuffer: 16 << 20 });
  const files = inventory.split(/\r?\n/).filter(Boolean);
  assert(files.length > 0, `${file}: missing compiler source inventory`);
  return new Set(files.map(name => realpathSync(resolve(dirname(file), name))));
}

export function sourceCoverageProblems(root: string, files: string[], covered: Set<string>): string[] {
  return files.flatMap(file => {
    if (/\.(?:[cm]?js|jsx)$/.test(file)) return [`Maintained JavaScript must be TypeScript: ${file}`];
    if (/\.(?:[cm]?ts|tsx)$/.test(file) && !covered.has(realpathSync(resolve(root, file)))) return [`TypeScript is outside all checked projects: ${file}`];
    return [];
  });
}
