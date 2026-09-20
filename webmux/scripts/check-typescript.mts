import { execFileSync } from 'node:child_process';
import { realpathSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const projects = [
  'webmux/backend/tsconfig.json',
  'webmux/frontend/tsconfig.json',
  'webmux/tsconfig.tools.json',
  'webmux/tsconfig.helpers.json',
  'webmux/tsconfig.e2e.json',
  'tests/backend/tsconfig.json',
  'webmux/tsconfig.frontend-tests.json',
];
const covered = new Set<string>();
for (const project of projects) {
  const path = resolve(root, project);
  const config = ts.readConfigFile(path, ts.sys.readFile);
  if (config.error) throw new Error(ts.flattenDiagnosticMessageText(config.error.messageText, '\n'));
  const parsed = ts.parseJsonConfigFileContent(config.config, ts.sys, dirname(path));
  if (parsed.errors.length) throw new Error(ts.formatDiagnosticsWithColorAndContext(parsed.errors, {
    getCanonicalFileName: name => name, getCurrentDirectory: () => root, getNewLine: () => '\n',
  }));
  if (!parsed.options.strict) throw new Error(`${project} must enable strict checking`);
  for (const file of parsed.fileNames) covered.add(realpathSync(file));
}
// Include untracked, nonignored sources so the check also protects local work.
const files = execFileSync('git', ['ls-files', '-z', '--cached', '--others', '--exclude-standard'],
  { cwd: root, encoding: 'utf8' }).split('\0').filter(Boolean);
const problems = files.flatMap(file => {
  if (/\.(?:[cm]?js|jsx)$/.test(file)) return [`Maintained JavaScript must be TypeScript: ${file}`];
  if (/\.(?:[cm]?ts|tsx)$/.test(file) && !covered.has(realpathSync(resolve(root, file)))) return [`TypeScript is outside all checked projects: ${file}`];
  return [];
});
if (problems.length) throw new Error(problems.join('\n'));
const compiler = createRequire(import.meta.url).resolve('typescript/bin/tsc');
for (const project of projects) {
  console.log(`Checking ${project}`);
  execFileSync(process.execPath, [compiler, '--noEmit', '--project', resolve(root, project)], { cwd: root, stdio: 'inherit' });
}
console.log('All maintained TypeScript sources are covered by strict compiler checks.');
