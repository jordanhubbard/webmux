import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { test } from 'node:test';

const source = path.resolve(import.meta.dirname, '..');
const unix = { skip: process.platform === 'win32' };

// Real Make recipes, release scripts, metadata and Git transitions; only the
// application test command and GitHub service are substituted. No network
// origin, operator Git configuration or actual publication is used.
function fixture() {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'webmux-release space-'));
  const repo = path.join(directory, 'source');
  const origin = path.join(directory, 'origin.git');
  const bin = path.join(directory, 'bin');
  const calls = path.join(directory, 'github-calls.jsonl');
  const tested = path.join(directory, 'tested.json');
  fs.mkdirSync(repo); fs.mkdirSync(bin);
  fs.mkdirSync(path.join(directory, 'empty-template'));
  const scratch = path.join(directory, 'scratch'); fs.mkdirSync(scratch);
  const env = {
    ...process.env, PATH: `${bin}${path.delimiter}${path.dirname(process.execPath)}${path.delimiter}${process.env.PATH ?? ''}`,
    GIT_CONFIG_GLOBAL: '/dev/null', GIT_CONFIG_NOSYSTEM: '1', GIT_TERMINAL_PROMPT: '0',
    GIT_CONFIG_COUNT: '0', GIT_TEMPLATE_DIR: path.join(directory, 'empty-template'),
    TMPDIR: scratch, TMP: scratch, TEMP: scratch,
    BATCH: 'yes', WEBMUX_FIXTURE_CALLS: calls, WEBMUX_FIXTURE_TESTED: tested,
  };
  function run(command: string, args: string[], extra: NodeJS.ProcessEnv = {}) {
    const result = spawnSync(command, args, { cwd: repo, env: { ...env, ...extra }, encoding: 'utf8', timeout: 30000 });
    assert.ifError(result.error);
    return result;
  }
  function git(...args: string[]): string {
    const result = run('git', args);
    assert.equal(result.status, 0, `${args.join(' ')}: ${result.stdout}${result.stderr}`);
    return result.stdout.trim();
  }
  const make = spawnSync('/bin/sh', ['-c', 'command -v make'], { encoding: 'utf8' }).stdout.trim();
  assert(make);
  for (const filename of ['Makefile', 'scripts/release.sh', 'scripts/release-metadata.mts',
    'scripts/release-readiness.mts', 'scripts/bootstrap-toolchain.sh']) {
    const destination = path.join(repo, filename);
    fs.mkdirSync(path.dirname(destination), { recursive: true });
    fs.copyFileSync(path.join(source, filename), destination);
  }
  function versions(version: string) {
    for (const filename of ['webmux/package.json', 'webmux/frontend/package.json']) {
      fs.mkdirSync(path.dirname(path.join(repo, filename)), { recursive: true });
      fs.writeFileSync(path.join(repo, filename), JSON.stringify({ name: 'release-fixture', version }) + '\n');
    }
    fs.writeFileSync(path.join(repo, 'webmux/package-lock.json'), JSON.stringify({ version, packages: {
      '': { version }, frontend: { version },
    } }) + '\n');
  }
  versions('1.2.3');
  fs.writeFileSync(path.join(repo, 'CHANGELOG.md'), '# Changelog\n\n## [Unreleased]\n');
  fs.writeFileSync(path.join(bin, 'make'), `#!/usr/bin/env node
const fs = require('node:fs');
const cp = require('node:child_process');
if (process.argv[2] !== 'test') process.exit(99);
const branch = cp.execFileSync('git', ['branch', '--show-current'], {encoding:'utf8'}).trim();
const version = JSON.parse(fs.readFileSync('webmux/package.json', 'utf8')).version;
fs.writeFileSync(process.env.WEBMUX_FIXTURE_TESTED, JSON.stringify({branch, version}));
console.log('Fixture application tests: ' + (process.env.WEBMUX_FIXTURE_TEST_EXIT ? 'failed' : 'passed'));
process.exit(Number(process.env.WEBMUX_FIXTURE_TEST_EXIT || 0));
`, { mode: 0o700 });
  fs.writeFileSync(path.join(bin, 'gh'), String.raw`#!/usr/bin/env node
const fs = require('node:fs');
const args = process.argv.slice(2);
fs.appendFileSync(process.env.WEBMUX_FIXTURE_CALLS, JSON.stringify(args) + '\n');
if (args[0] === 'auth') process.exit(Number(process.env.WEBMUX_FIXTURE_AUTH_EXIT || 0));
if (args[0] === 'run' && args[1] === 'list') {
  const cp = require('node:child_process');
  const names = {'ci.yml':'CI','packaging.yml':'Packaging','windows-arm-native.yml':'Windows ARM64 native qualification'};
  const file = args[args.indexOf('--workflow')+1];
  if (!names[file] || args[args.indexOf('--branch')+1] !== 'main') process.exit(97);
  const [selected, configured] = (process.env.WEBMUX_FIXTURE_RUN_MODE || ':success').split(':');
  const mode = !selected || selected === file ? configured : 'success';
  if (mode === 'advance') {
    fs.writeFileSync('concurrent-main.txt', 'independent committed main change');
    cp.execFileSync('git', ['add', 'concurrent-main.txt']);
    cp.execFileSync('git', ['commit', '-m', 'Concurrent main change']);
    cp.execFileSync('git', ['push'], {stdio:'ignore'});
  }
  if (mode === 'query-error') process.exit(1);
  if (mode === 'malformed') { console.log('{broken'); process.exit(0); }
  const run = {databaseId:100, workflowName:names[file], headSha:args[args.indexOf('--commit')+1], headBranch:'main',
    event:'push', createdAt:'2026-01-01T00:00:00Z', status:'completed', conclusion:mode === 'advance' ? 'success' : mode};
  if (mode === 'stale') { run.headSha = '0'.repeat(40); run.conclusion = 'success'; }
  if (mode === 'pending') { run.status = 'in_progress'; run.conclusion = null; }
  console.log(JSON.stringify(mode === 'missing' ? [] : [run])); process.exit(0);
}
if (args[0] === 'repo') { console.log('https://example.test/fixture'); process.exit(0); }
if (args[0] === 'pr' && args[1] === 'create') {
  fs.copyFileSync(args[args.indexOf('--body-file')+1], process.env.WEBMUX_FIXTURE_CALLS + '.pr-body');
  console.log('https://example.test/fixture/pull/1'); process.exit(0);
}
if (args[0] === 'release' && args[1] === 'create') {
  fs.copyFileSync(args[args.indexOf('--notes-file')+1], process.env.WEBMUX_FIXTURE_CALLS + '.release-notes');
  process.exit(Number(process.env.WEBMUX_FIXTURE_PUBLISH_EXIT || 0));
}
process.exit(98);
`, { mode: 0o700 });
  git('init', '--bare', origin);
  git('init', '-b', 'main');
  git('config', 'user.name', 'Release fixture');
  git('config', 'user.email', 'release@example.test');
  git('config', 'commit.gpgsign', 'false');
  git('config', 'tag.gpgsign', 'false');
  git('config', 'core.hooksPath', '/dev/null');
  git('add', '.'); git('commit', '-m', 'feat: initial fixture');
  git('tag', 'v1.2.3');
  git('remote', 'add', 'origin', origin);
  git('push', '-u', 'origin', 'main'); git('push', 'origin', 'v1.2.3');
  const initial = git('rev-parse', 'main');
  return {
    repo, initial, git, versions,
    release: (target: string, extra: NodeJS.ProcessEnv = {}) => run(make, ['--no-print-directory', target, `NODE=${process.execPath}`], extra),
    calls: () => fs.existsSync(calls) ? fs.readFileSync(calls, 'utf8').trim().split('\n').map(line => JSON.parse(line) as string[]) : [],
    tested: () => JSON.parse(fs.readFileSync(tested, 'utf8')) as { branch: string; version: string },
    body: (kind: 'pr-body' | 'release-notes') => fs.readFileSync(calls + '.' + kind, 'utf8'),
    cleanup: () => fs.rmSync(directory, { recursive: true, force: true }),
  };
}

test('release Make targets prepare patch, minor and major PR branches without changing main', unix, () => {
  for (const [target, version] of [['release', '1.2.4'], ['release-minor', '1.3.0'], ['release-major', '2.0.0']]) {
    const f = fixture();
    try {
      const result = f.release(target);
      assert.equal(result.status, 0, result.stdout + result.stderr);
      assert.equal(f.git('branch', '--show-current'), 'main');
      assert.equal(f.git('status', '--porcelain'), '');
      assert.equal(f.git('rev-parse', 'main'), f.initial);
      assert.deepEqual(f.tested(), { branch: `release/v${version}`, version });
      assert.equal(JSON.parse(f.git('show', `release/v${version}:webmux/package.json`)).version, version);
      assert(f.git('ls-remote', 'origin', `refs/heads/release/v${version}`));
      assert.match(f.body('pr-body'), new RegExp(`Release v${version.replaceAll('.', '\\.')}`));
      assert.equal(f.calls().filter(call => call[0] === 'pr' && call[1] === 'create').length, 1);
      assert(!f.calls().some(call => call[0] === 'release'));
      assert.equal(f.git('tag', '--list'), 'v1.2.3');
    } finally { f.cleanup(); }
  }
});

test('failed release tests preserve the release branch and edits without pushing or opening a PR', unix, () => {
  const f = fixture();
  try {
    const result = f.release('release', { WEBMUX_FIXTURE_TEST_EXIT: '7' });
    assert.notEqual(result.status, 0);
    assert.match(result.stdout + result.stderr, /Tests failed/);
    assert.equal(f.git('branch', '--show-current'), 'release/v1.2.4');
    assert.equal(f.git('rev-parse', 'main'), f.initial);
    assert.match(f.git('status', '--porcelain'), /CHANGELOG.md/);
    assert.equal(JSON.parse(fs.readFileSync(path.join(f.repo, 'webmux/package.json'), 'utf8')).version, '1.2.4');
    assert.equal(f.git('ls-remote', 'origin', 'refs/heads/release/v1.2.4'), '');
    assert(!f.calls().some(call => call[0] === 'pr' || call[0] === 'release'));
  } finally { f.cleanup(); }
});

test('dirty and non-main preparation refuse before modifying refs or invoking tests', unix, () => {
  for (const dirty of [false, true]) {
    const f = fixture();
    try {
      if (dirty) fs.appendFileSync(path.join(f.repo, 'CHANGELOG.md'), '\nretained operator edit\n');
      else f.git('switch', '-c', 'feature/fixture');
      const before = f.git('show-ref');
      const result = f.release('release');
      assert.notEqual(result.status, 0);
      assert.match(result.stdout + result.stderr, dirty ? /not clean/ : /Not on main/);
      assert.equal(f.git('show-ref'), before);
      assert(!f.calls().some(call => call[0] === 'pr' || call[0] === 'release'));
    } finally { f.cleanup(); }
  }
});

test('release-publish binds its annotated tag to current main and passes notes to GitHub', unix, () => {
  const f = fixture();
  try {
    f.versions('1.2.4'); f.git('add', '.'); f.git('commit', '-m', 'Release v1.2.4'); f.git('push');
    const head = f.git('rev-parse', 'main');
    const result = f.release('release-publish');
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.equal(f.git('cat-file', '-t', 'v1.2.4'), 'tag');
    assert.equal(f.git('rev-parse', 'v1.2.4^{commit}'), head);
    assert.match(f.git('ls-remote', 'origin', 'refs/tags/v1.2.4^{}'), new RegExp(`^${head}`));
    assert.match(f.body('release-notes'), /WebMux v1\.2\.4/);
    assert.equal(f.calls().filter(call => call[0] === 'release' && call[1] === 'create').length, 1);
    assert.equal(f.calls().filter(call => call[0] === 'run' && call[1] === 'list').length, 3);
    assert.equal(f.git('status', '--porcelain'), '');
    assert.notEqual(f.release('release-publish').status, 0, 'Existing tag must refuse duplicate publication');
    assert.equal(f.calls().filter(call => call[0] === 'release' && call[1] === 'create').length, 1);
  } finally { f.cleanup(); }
});

test('release-publish refuses unqualified workflow runs before creating or pushing a tag', unix, () => {
  for (const mode of ['ci.yml:missing', 'ci.yml:pending', 'packaging.yml:failure',
    'packaging.yml:cancelled', 'windows-arm-native.yml:timed_out', 'windows-arm-native.yml:skipped',
    'ci.yml:stale', 'ci.yml:query-error', 'ci.yml:malformed']) {
    const f = fixture();
    try {
      f.versions('1.2.4'); f.git('add', '.'); f.git('commit', '-m', 'Release v1.2.4'); f.git('push');
      const before = f.git('show-ref');
      const result = f.release('release-publish', { WEBMUX_FIXTURE_RUN_MODE: mode });
      assert.notEqual(result.status, 0, mode);
      assert.equal(f.git('show-ref'), before, `${mode} changed refs`);
      assert.equal(f.git('ls-remote', 'origin', 'refs/tags/v1.2.4'), '', `${mode} pushed a tag`);
      assert(!f.calls().some(call => call[0] === 'release'), `${mode} attempted GitHub publication`);
    } finally { f.cleanup(); }
  }
});

test('publication refuses a main change after readiness instead of tagging an unqualified revision', unix, () => {
  const f = fixture();
  try {
    f.versions('1.2.4'); f.git('add', '.'); f.git('commit', '-m', 'Release v1.2.4'); f.git('push');
    const before = f.git('rev-parse', 'main');
    const result = f.release('release-publish', { WEBMUX_FIXTURE_RUN_MODE: 'ci.yml:advance' });
    assert.notEqual(result.status, 0);
    assert.match(result.stdout + result.stderr, /Main changed during release validation/);
    assert.notEqual(f.git('rev-parse', 'main'), before, 'Fixture did not exercise a concurrent main change');
    assert.equal(f.git('tag', '--list'), 'v1.2.3');
    assert.equal(f.git('ls-remote', 'origin', 'refs/tags/v1.2.4'), '');
    assert(!f.calls().some(call => call[0] === 'release'));
  } finally { f.cleanup(); }
});
