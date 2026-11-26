#!/usr/bin/env node
// Admission for publication; this does not replace roadmap or release review.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

export const releaseWorkflows = [
  { file: 'ci.yml', name: 'CI' },
  { file: 'packaging.yml', name: 'Packaging' },
  { file: 'windows-arm-native.yml', name: 'Windows ARM64 native qualification' },
] as const;

export function requireWorkflowSuccess(data: unknown, head: string, workflow: string): void {
  assert(Array.isArray(data) && data.length > 0 && data.length <= 100, `${workflow}: missing or invalid exact-main workflow runs`);
  const ids = new Set<number>();
  const runs = data.map((value: unknown) => {
    assert(value && typeof value === 'object' && !Array.isArray(value), `${workflow}: invalid workflow run`);
    const run = value as Record<string, unknown>;
    assert(run.workflowName === workflow && run.headSha === head && run.headBranch === 'main'
      && (run.event === 'push' || run.event === 'workflow_dispatch'), `${workflow}: run does not qualify this exact main revision`);
    assert(typeof run.databaseId === 'number' && Number.isSafeInteger(run.databaseId) && run.databaseId > 0
      && !ids.has(run.databaseId), `${workflow}: invalid or duplicated workflow run identity`);
    ids.add(run.databaseId);
    assert(typeof run.createdAt === 'string' && Number.isFinite(Date.parse(run.createdAt)), `${workflow}: missing workflow ordering evidence`);
    assert(typeof run.status === 'string' && (run.conclusion === null || typeof run.conclusion === 'string'), `${workflow}: invalid workflow state`);
    return { id: run.databaseId, created: Date.parse(run.createdAt), status: run.status, conclusion: run.conclusion };
  });
  runs.sort((a, b) => b.created - a.created || b.id - a.id);
  const latest = runs[0];
  assert(latest.status === 'completed' && latest.conclusion === 'success',
    `${workflow}: latest exact-main run ${latest.id} is ${latest.status}/${latest.conclusion ?? 'no conclusion'}; successful completion is required`);
}

function main(): void {
  const [head, ...extra] = process.argv.slice(2);
  assert(head && /^[0-9a-f]{40}$/.test(head) && !extra.length, 'Usage: release-readiness.mts <exact main SHA>');
  for (const workflow of releaseWorkflows) {
    const result = spawnSync('gh', ['run', 'list', '--workflow', workflow.file, '--branch', 'main', '--commit', head,
      '--limit', '100', '--json', 'databaseId,workflowName,headSha,headBranch,event,createdAt,status,conclusion'], {
      encoding: 'utf8', timeout: 20000, maxBuffer: 4 << 20,
    });
    if (result.error) throw new Error(`${workflow.name}: cannot inspect release readiness: ${result.error.message}`);
    assert(result.status === 0, `${workflow.name}: workflow query failed: ${result.stderr.trim().slice(0, 1000)}`);
    requireWorkflowSuccess(JSON.parse(result.stdout), head, workflow.name);
    console.log(`${workflow.name}: latest exact-main run succeeded at ${head}`);
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) main();
