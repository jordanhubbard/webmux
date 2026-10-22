import { beforeEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { CoordinationDialog } from '@frontend/components/CoordinationDialog';
const mocks = vi.hoisted(() => ({ status: vi.fn(), create: vi.fn(), attach: vi.fn() }));
vi.mock('@frontend/utils/api', () => ({ api: { getCoordination: mocks.status, createCoordinatedAgent: mocks.create, attachCoordinatedAgent: mocks.attach } }));
beforeEach(() => {
  vi.clearAllMocks();
  mocks.status.mockResolvedValue({ agents: [], messages: [] });
  mocks.create.mockResolvedValue({});
  mocks.attach.mockResolvedValue({});
});
it('opens without launching agents and reports coordinator failures', async () => {
  mocks.status.mockRejectedValue(new Error('Install OpenRig'));
  render(<CoordinationDialog onClose={() => {}} />);
  expect(await screen.findByRole('alert')).toHaveTextContent('Install OpenRig');
  expect(mocks.create).not.toHaveBeenCalled();
  expect(mocks.attach).not.toHaveBeenCalled();
});
it('creates an agent in the selected host directory and refreshes the ordinary grid', async () => {
  const changed = vi.fn(); const close = vi.fn();
  window.addEventListener('webmux-sessions-changed', changed);
  try {
    render(<CoordinationDialog onClose={close} />);
    fireEvent.change(screen.getByLabelText('Project ID'), { target: { value: 'shared-project' } });
    fireEvent.change(screen.getByLabelText('Agent name'), { target: { value: 'reviewer' } });
    fireEvent.change(screen.getByLabelText('Execution host'), { target: { value: 'sparky.local' } });
    fireEvent.change(screen.getByLabelText('Project directory on this host'), { target: { value: '/work/project' } });
    expect(screen.getByRole('combobox', { name: 'Agent' })).toHaveValue('codex');
    fireEvent.click(screen.getByRole('button', { name: 'Create and open terminal' }));
    await waitFor(() => expect(close).toHaveBeenCalledOnce());
    expect(mocks.create).toHaveBeenCalledWith({ project: 'shared-project', name: 'reviewer', host: 'sparky.local', cwd: '/work/project', runtime: 'codex' });
    expect(changed).toHaveBeenCalledOnce();
  } finally { window.removeEventListener('webmux-sessions-changed', changed); }
});
it('shows attributed coordination and opens an existing agent without relaunching', async () => {
  mocks.status.mockResolvedValue({ agents: [{ target: 'seat', name: 'reviewer', project: 'shared', host: 'remote', cwd: '/work', runtime: 'codex', connected: true }], messages: [{ from: 'seat', to: 'peer', time: '2026-10-21T16:00:00Z', state: 'pending', body: 'Fresh peer reply' }] });
  render(<CoordinationDialog onClose={() => {}} />);
  expect(await screen.findByText('Fresh peer reply')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Open terminal' }));
  await waitFor(() => expect(mocks.attach).toHaveBeenCalledWith('seat'));
  expect(mocks.create).not.toHaveBeenCalled();
});

it('groups workers by explicit project and preserves reconnect actions', async () => {
  const agents = [
    { target: 'opaque-one', project: 'demo-release', name: 'reviewer', host: 'build-host', runtime: 'codex', cwd: '/work', connected: false },
    { target: 'opaque-two', project: 'demo-release', name: 'coder', host: 'localhost', runtime: 'claude-code', cwd: '/work', connected: true, activityState: 'working' },
    { target: 'opaque-three', project: 'other-project', name: 'reviewer', host: 'other-host', runtime: 'codex', cwd: '/work', connected: true },
  ];
  mocks.status.mockResolvedValue({ agents, messages: [] });
  render(<CoordinationDialog onClose={() => {}} />);
  const project = await screen.findByRole('list', { name: 'demo-release workers' });
  expect(within(project).getAllByRole('article')).toHaveLength(2);
  expect(screen.getByRole('list', { name: 'other-project workers' })).toBeVisible();
  expect(within(project).getByText('working')).toBeVisible();
  const group = project.closest('details')!;
  expect(group.open).toBe(true);
  fireEvent.click(group.querySelector('summary')!);
  expect(group.open).toBe(false);
  fireEvent.click(group.querySelector('summary')!);
  fireEvent.click(within(project).getByRole('button', { name: 'Reconnect agent' }));
  await waitFor(() => expect(mocks.attach).toHaveBeenCalledWith('opaque-one'));
  expect(mocks.create).toHaveBeenCalledWith(agents[0]);
});
