import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { checkedProjectSources, sourceCoverageProblems } from './typescript-project.mts';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const projects = [
  'webmux/frontend/tsconfig.json',
  'webmux/tsconfig.tools.json',
  'webmux/tsconfig.helpers.json',
  'webmux/tsconfig.e2e.json',
  'webmux/tsconfig.frontend-tests.json',
];
const compiler = createRequire(import.meta.url).resolve('typescript/bin/tsc');
const covered = new Set<string>();
for (const project of projects) {
  for (const file of checkedProjectSources(compiler, resolve(root, project))) covered.add(file);
}
// Include untracked, nonignored sources so the check also protects local work.
const files = execFileSync('git', ['ls-files', '-z', '--cached', '--others', '--exclude-standard'],
  { cwd: root, encoding: 'utf8' }).split('\0').filter(Boolean);
const problems = sourceCoverageProblems(root, files, covered);
if (problems.length) throw new Error(problems.join('\n'));
for (const project of projects) {
  console.log(`Checking ${project}`);
  execFileSync(process.execPath, [compiler, '--noEmit', '--project', resolve(root, project)], { cwd: root, stdio: 'inherit' });
}
console.log('All maintained TypeScript sources are covered by strict compiler checks.');
