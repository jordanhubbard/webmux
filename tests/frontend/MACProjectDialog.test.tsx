import { createRef } from 'react';
import { afterEach, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { MACProjectDialog } from '@frontend/components/MACProjectDialog';
import { TerminalActions } from '@frontend/components/TerminalActions';
import type { TerminalHandle } from '@frontend/components/Terminal';
import { api, type MACProjectView } from '@frontend/utils/api';

function observation(id = 'one'): MACProjectView {
  return {
    schema: 'webmux.mac.session-project-view.v1', subject: { node_id: 'node', owner: 'owner', session_id: id, incarnation: 'run' }, standalone_history_state: 'retained_unreconciled',
    source_state: 'observed', source_task_link_state: 'unlinked', source: { schema: 'webmux.mac.source-observation.v1', worktree: `/work/${id}`, ref: 'refs/heads/topic', commit: 'a'.repeat(40), detached: false, dirty: true, observed_at: '2026-11-23T07:00:00Z' },
    roadmap_state: 'observed_committed_snapshot', roadmap: { schema: 'webmux.mac.source-roadmap-observation.v1', path: 'docs/roadmap.md', commit: 'a'.repeat(40), blob: 'b'.repeat(40), sha256: 'c'.repeat(64), text: '# Source intent\n<script>execute()</script>', working_copy_dirty: true, task_link_state: 'unlinked', observed_at: '2026-11-23T07:00:00Z' },
    project: { schema: 'webmux.mac.project-observation.v1', project_key: `project-${id}`, observed_at: '2026-11-23T07:00:00Z', coverage: 'incomplete', reported_total: 4, next_offset: 1,
      conflict_verdict: 'unknown', ordering_verdict: 'unknown', unavailable_tasks: 1, unavailable_workers: 1, adapter_omitted_tasks: 0, upstream_omitted_tasks: 0, withheld_dependencies: 1,
      tasks: [{ task: { task_id: 'task-one', title: '<img src=x onerror="execute()">', state: 'running', priority: 10, attempt_count: 2, lease_observation: 'observed_unexpired', leased_until: '2026-11-23T08:00:00Z' }, worker: { worker_id: 'canonical-worker', worker_name: 'Rocky', machine_id: 'canonical-machine', machine_hostname: 'puck', availability: 'observed' }, worker_state: 'observed' }], dependencies: [] },
    sessions: [{ session: { node_id: 'node', session_id: id, incarnation: 'run' }, execution_host: 'puck.local', transport: 'local', availability: 'present', mac_link_state: 'unlinked', observed_at: '2026-11-23T07:00:00Z' }],
  };
}
afterEach(() => vi.restoreAllMocks());

it('loads from the terminal action only when requested and displays qualified task and session facts as text', async () => {
  const load = vi.spyOn(api, 'getMACProject').mockResolvedValue(observation());
  render(<TerminalActions sessionId="one" terminalRef={createRef<TerminalHandle>()} transcriptEnabled={false} connected={false} />);
  expect(load).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Project activity' }));
  expect(await screen.findByText('project-one')).toBeDefined();
  expect(screen.getByText('<img src=x onerror="execute()">')).toBeDefined();
  expect(document.querySelector('img')).toBeNull();
  expect(screen.getByText(/Rocky \(canonical-worker\)/)).toBeDefined();
  expect(screen.getByText(/puck.local.*MAC task link: unlinked/)).toBeDefined();
  expect(screen.getByText(/Showing 1 tasks.*reports 4/)).toBeDefined();
  expect(screen.getByText(/Some tasks or worker observations/)).toBeDefined();
  expect(screen.getByText(/Assessment: unknown. Task scope/)).toBeDefined();
  expect(screen.getByText(/Assessment: unknown. The source roadmap/)).toBeDefined();
  expect(screen.getByText(/Worktree: \/work\/one/)).toBeDefined();
  expect(screen.getByText(/Uncommitted or untracked changes observed/)).toBeDefined();
  expect(screen.getByText(/not linked to a MAC task or roadmap revision/)).toBeDefined();
  expect(screen.getByLabelText('Source roadmap text').textContent).toBe(observation().roadmap!.text);
  expect(document.querySelector('script')).toBeNull();
  expect(screen.getByText(/Working-copy changes to this roadmap are not included/)).toBeDefined();
  expect(screen.getByText(/does not verify issue status or grant admission/)).toBeDefined();
  expect(load).toHaveBeenCalledOnce();
});

it('removes prior facts on refresh failure and does not retain a successful project view', async () => {
  vi.spyOn(api, 'getMACProject').mockResolvedValueOnce(observation()).mockRejectedValueOnce(new Error('MAC access revoked'));
  render(<MACProjectDialog sessionId="one" onClose={vi.fn()} />);
  expect(await screen.findByText('project-one')).toBeDefined();
  fireEvent.click(screen.getByRole('button', { name: 'Refresh project' }));
  expect(screen.queryByText('project-one')).toBeNull();
  expect(await screen.findByRole('alert')).toHaveTextContent('MAC access revoked');
  expect(screen.queryByText('task-one')).toBeNull();
  expect(screen.queryByText(/Worktree: \/work\/one/)).toBeNull();
  expect(screen.queryByLabelText('Source roadmap text')).toBeNull();
});

it('shows unavailable remote source context without borrowing a local path or commit', async () => {
  const value = observation(); value.source = null; value.source_state = 'execution_host_unavailable';
  value.roadmap = null; value.roadmap_state = 'execution_host_unavailable';
  vi.spyOn(api, 'getMACProject').mockResolvedValue(value);
  render(<MACProjectDialog sessionId="one" onClose={vi.fn()} />);
  expect(await screen.findByText(/Source inspection on this execution host is unavailable/)).toBeDefined();
  expect(screen.queryByText(/Worktree:/)).toBeNull();
  expect(screen.queryByText('a'.repeat(40))).toBeNull();
  expect(screen.queryByLabelText('Source roadmap text')).toBeNull();
});

it('discards a late response after selecting a different session', async () => {
  let finish!: (value: MACProjectView) => void;
  vi.spyOn(api, 'getMACProject').mockImplementationOnce(() => new Promise(resolve => { finish = resolve; })).mockResolvedValueOnce(observation('two'));
  const view = render(<MACProjectDialog sessionId="one" onClose={vi.fn()} />);
  view.rerender(<MACProjectDialog sessionId="two" onClose={vi.fn()} />);
  expect(await screen.findByText('project-two')).toBeDefined();
  await act(async () => finish(observation('one')));
  expect(screen.queryByText('project-one')).toBeNull();
  expect(screen.getByText('project-two')).toBeDefined();
});

it('requests a fresh bounded next page rather than appending it as an atomic snapshot', async () => {
  const next = observation(); next.project.tasks = []; next.project.next_offset = null;
  const load = vi.spyOn(api, 'getMACProject').mockResolvedValueOnce(observation()).mockResolvedValueOnce(next);
  render(<MACProjectDialog sessionId="one" onClose={vi.fn()} />);
  fireEvent.click(await screen.findByRole('button', { name: 'Next task page' }));
  expect(await screen.findByText(/No tasks are visible on this page/)).toBeDefined();
  expect(screen.queryByText('task-one')).toBeNull();
  expect(load.mock.calls[1][1]).toBe(1);
  expect(screen.getByText(/Each page is a fresh observation/)).toBeDefined();
});
