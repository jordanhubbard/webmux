import { beforeEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { CoordinationDialog, type CoordinationStatus } from '@frontend/components/CoordinationDialog';

const mocks = vi.hoisted(() => ({ status: vi.fn(), create: vi.fn(), attach: vi.fn(), browse: vi.fn(), updateConfig: vi.fn(), send: vi.fn(), sendStatus: vi.fn(), drain: vi.fn() }));
vi.mock('@frontend/utils/api', () => ({ api: {
  getCoordination: mocks.status,
  createCoordinatedAgent: mocks.create,
  attachCoordinatedAgent: mocks.attach,
  browseCoordinationWorkspaces: mocks.browse,
  updateConfig: mocks.updateConfig,
  sendCoordinationMessage: mocks.send,
  getCoordinationSendStatus: mocks.sendStatus,
  drainCoordinationRemote: mocks.drain,
} }));

const emptyStatus = (): CoordinationStatus => ({
  contract: 'webmux-coordination/v1',
  defaultProvider: 'rig',
  providers: [
    { id: 'rig', label: 'Rig', available: true, capabilities: ['projects', 'topology', 'launch', 'attach'], contract: 'openrig-cli/0.6.14' },
    { id: 'agent-deck', label: 'Agent Deck', available: false, message: 'Install Agent Deck', capabilities: ['sessions'], contract: 'agent-deck-cli/v1' },
  ],
  projects: [], seats: [], messages: [], errors: {},
});

beforeEach(() => {
  vi.clearAllMocks();
  mocks.status.mockResolvedValue(emptyStatus());
  mocks.create.mockResolvedValue({ target: 'new-seat', session: { id: 'viewer' } });
  mocks.attach.mockResolvedValue({ id: 'viewer' });
  mocks.browse.mockResolvedValue({ host: 'localhost', path: '/work/project', parent: '/work', repository: true, entries: [] });
  mocks.updateConfig.mockResolvedValue({ app: { coordination: { default_provider: 'agent-deck' } } });
  mocks.send.mockResolvedValue({ target: 'sparky:session-1', sendId: 'send-1', state: 'queued', queued: true, submitted: true });
  mocks.sendStatus.mockResolvedValue({ state: 'landed', confirmation: 'accepted', submitted: true });
  mocks.drain.mockResolvedValue({ remote: 'sparky', target: 'local:conductor-1', fetched: 2, written: 1, duplicates: 1, unknown: 0, woke: true, messages: [] });
});

it('opens without launching sessions and reports provider failures', async () => {
  mocks.status.mockRejectedValue(new Error('Cannot read coordinators'));
  render(<CoordinationDialog onClose={() => {}} />);
  expect(await screen.findByRole('alert')).toHaveTextContent('Cannot read coordinators');
  expect(mocks.create).not.toHaveBeenCalled();
  expect(mocks.attach).not.toHaveBeenCalled();
});

it('shows Rig diagnostics only while using the Rig provider', async () => {
  const view = render(<CoordinationDialog onClose={() => {}} />);
  fireEvent.click(await screen.findByText('Rig diagnostics'));
  expect(screen.getByRole('link', { name: /Open Rig text dashboard/ })).toHaveAttribute('href', '?panel=openrig');
  view.unmount();
  mocks.status.mockResolvedValue({ ...emptyStatus(), defaultProvider: 'agent-deck' });
  render(<CoordinationDialog onClose={() => {}} />);
  await screen.findByText('Using');
  expect(screen.queryByRole('link', { name: /Open Rig text dashboard/ })).toBeNull();
});

it('offers None alongside both providers and persists it without creating sessions', async () => {
  render(<CoordinationDialog onClose={() => {}} />);
  await screen.findByText('Using');
  fireEvent.change(screen.getByLabelText('Coordinator'), { target: { value: 'disabled' } });
  expect(screen.getByRole('status')).toHaveTextContent('No coordinator selected');
  expect(screen.queryByText('Start a coding agent')).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Make default' }));
  await waitFor(() => expect(mocks.updateConfig).toHaveBeenCalledWith({ app: { coordination: { default_provider: 'disabled' } } }));
  expect(mocks.create).not.toHaveBeenCalled();
});

it('launches from the browsed folder without requiring a second folder selection', async () => {
  const changed = vi.fn(); const close = vi.fn();
  window.addEventListener('webmux-sessions-changed', changed);
  try {
    render(<CoordinationDialog onClose={close} />);
    fireEvent.click(await screen.findByLabelText('Start a new project'));
    fireEvent.change(screen.getByLabelText('Project name'), { target: { value: 'shared-project' } });
    fireEvent.change(screen.getByLabelText('Session name'), { target: { value: 'reviewer' } });
    fireEvent.click(screen.getByRole('button', { name: 'Browse' }));
    await waitFor(() => expect(mocks.browse).toHaveBeenCalledWith('rig', 'localhost', undefined));
    await waitFor(() => expect(screen.getByLabelText('Code folder')).toHaveValue('/work/project'));
    mocks.browse.mockResolvedValue({ host: 'localhost', path: '/work', parent: '/', repository: false, entries: [] });
    fireEvent.click(screen.getByRole('button', { name: 'Parent folder' }));
    await waitFor(() => expect(screen.getByLabelText('Code folder')).toHaveValue('/work'));
    fireEvent.click(screen.getByRole('button', { name: 'Start agent and open terminal' }));
    await waitFor(() => expect(close).toHaveBeenCalledOnce());
    expect(mocks.create).toHaveBeenCalledWith({ provider: 'rig', mode: 'new', projectId: undefined, projectName: 'shared-project', context: undefined, name: 'reviewer', host: 'localhost', workspace: '/work', runtime: 'codex' });
    expect(mocks.attach).not.toHaveBeenCalled();
    expect(changed).toHaveBeenCalledOnce();
  } finally { window.removeEventListener('webmux-sessions-changed', changed); }
});

it('uses canonical Rig topology and state instead of stale private project records', async () => {
  const value = emptyStatus();
  value.projects = [
    { provider: 'rig', id: 'active-id', name: 'canonical-active', host: 'localhost', state: 'running', historical: false, seatCount: 1, runningCount: 1, contexts: ['agents'] },
    { provider: 'rig', id: 'stopped-id', name: 'canonical-stopped', host: 'localhost', state: 'stopped', historical: true, seatCount: 0, runningCount: 0 },
  ];
  value.seats = [{ provider: 'rig', id: 'canonical-seat', projectId: 'active-id', projectName: 'canonical-active', name: 'coder', host: 'sparky', workspace: '/src/project', runtime: 'codex', state: 'running', openable: true, managed: true, context: 'agents' }];
  mocks.status.mockResolvedValue(value);
  render(<CoordinationDialog onClose={() => {}} />);
  const project = await screen.findByRole('list', { name: 'canonical-active sessions' });
  expect(within(project).getByText('/src/project')).toBeVisible();
  expect(screen.getByText('canonical-stopped')).toBeVisible();
  expect(screen.queryByText('trickle-release')).not.toBeInTheDocument();
  fireEvent.click(within(project).getByRole('button', { name: 'Open session' }));
  await waitFor(() => expect(mocks.attach).toHaveBeenCalledWith('rig', 'canonical-seat', undefined));
});

it('creates a seat in an existing canonical Rig pod', async () => {
  const value = emptyStatus();
  value.projects = [{ provider: 'rig', id: 'rig-id', name: 'shared-rig', host: 'localhost', state: 'running', historical: false, seatCount: 1, runningCount: 1, contexts: ['reviewers', 'workers'] }];
  value.seats = [{ provider: 'rig', id: 'seat', projectId: 'rig-id', projectName: 'shared-rig', name: 'owner', host: 'localhost', workspace: '/work', runtime: 'codex', state: 'running', openable: true, managed: false, context: 'workers' }];
  mocks.status.mockResolvedValue(value);
  render(<CoordinationDialog onClose={() => {}} />);
  await screen.findByText('shared-rig');
  fireEvent.click(screen.getByLabelText('Add an agent to an existing project'));
  fireEvent.change(screen.getByLabelText('Rig pod'), { target: { value: 'reviewers' } });
  fireEvent.change(screen.getByLabelText('Session name'), { target: { value: 'reviewer' } });
  fireEvent.change(screen.getByLabelText('Code folder'), { target: { value: '/work' } });
  fireEvent.click(screen.getByRole('button', { name: 'Start agent and open terminal' }));
  await waitFor(() => expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({ provider: 'rig', mode: 'existing', projectId: 'rig-id', projectName: 'shared-rig', context: 'reviewers', workspace: '/work' })));
});

it('surfaces Agent Deck local and remote sessions with host-qualified identity', async () => {
  const value = emptyStatus();
  value.providers[1] = { id: 'agent-deck', label: 'Agent Deck', available: true, capabilities: ['sessions', 'remote', 'native-session-id'], contract: 'agent-deck-cli/v1' };
  value.defaultProvider = 'agent-deck';
  value.projects = [{ provider: 'agent-deck', id: 'sparky:webmux', name: 'webmux', host: 'sparky', state: 'active', historical: false, seatCount: 1, runningCount: 1 }];
  value.seats = [{ provider: 'agent-deck', id: 'sparky:session-1', projectId: 'sparky:webmux', projectName: 'webmux', name: 'remote-review', host: 'sparky', workspace: '/home/jkh/Src/webmux', runtime: 'codex', state: 'running', openable: true, managed: true, nativeSessionId: 'codex-thread-1' }];
  mocks.status.mockResolvedValue(value);
  render(<CoordinationDialog onClose={() => {}} />);
  const sessions = await screen.findByRole('list', { name: 'webmux sessions' });
  expect(within(sessions).getByText('codex-thread-1')).toBeVisible();
  expect(within(sessions).getByText('/home/jkh/Src/webmux')).toBeVisible();
  fireEvent.click(within(sessions).getByRole('button', { name: 'Open session' }));
  await waitFor(() => expect(mocks.attach).toHaveBeenCalledWith('agent-deck', 'sparky:session-1', undefined));
});

it('queues Agent Deck instructions and collects remote completions into a conductor', async () => {
  const value = emptyStatus();
  value.providers[1] = { id: 'agent-deck', label: 'Agent Deck', available: true, capabilities: ['sessions', 'remote', 'parent-child', 'send', 'queue', 'send-status', 'completion-inbox', 'remote-drain'], contract: 'agent-deck-cli/v1' };
  value.defaultProvider = 'agent-deck';
  value.attentionCount = 1;
  value.projects = [
    { provider: 'agent-deck', id: 'local:webmux', name: 'webmux', host: 'localhost', state: 'active', historical: false, seatCount: 1, runningCount: 1, attentionCount: 1 },
    { provider: 'agent-deck', id: 'sparky:webmux', name: 'webmux', host: 'sparky', state: 'active', historical: false, seatCount: 1, runningCount: 1 },
  ];
  value.seats = [
    { provider: 'agent-deck', id: 'local:conductor-1', projectId: 'local:webmux', projectName: 'webmux', name: 'conductor', host: 'localhost', runtime: 'codex', state: 'running', openable: true, managed: true, role: 'conductor', attention: true, canReceive: true },
    { provider: 'agent-deck', id: 'sparky:worker-1', projectId: 'sparky:webmux', projectName: 'webmux', name: 'worker', host: 'sparky', runtime: 'claude-code', state: 'running', openable: true, managed: true, role: 'worker', parentId: 'parent-1', parentName: 'conductor', canReceive: true },
  ];
  value.messages = [{ provider: 'agent-deck', from: 'worker', to: 'conductor', body: 'Review complete', time: '2026-10-26T10:00:00Z', state: 'completed', kind: 'completion', sourceRemote: 'sparky' }];
  mocks.status.mockResolvedValue(value);

  render(<CoordinationDialog onClose={() => {}} />);
  const remote = await screen.findByRole('article', { name: 'worker in webmux' });
  expect(within(remote).getByText('conductor')).toBeVisible();
  fireEvent.click(within(remote).getByRole('button', { name: 'Send instruction' }));
  fireEvent.change(within(remote).getByLabelText('Instruction for worker'), { target: { value: 'Run the focused review' } });
  fireEvent.click(within(remote).getByRole('button', { name: 'Queue instruction' }));
  await waitFor(() => expect(mocks.send).toHaveBeenCalledWith('agent-deck', 'sparky:worker-1', 'Run the focused review', true));
  expect(await within(remote).findByRole('status')).toHaveTextContent('queued');

  fireEvent.click(screen.getByRole('button', { name: 'Collect completions' }));
  await waitFor(() => expect(mocks.drain).toHaveBeenCalledWith('agent-deck', 'sparky', 'local:conductor-1'));
  expect(await screen.findByText(/Collected 1 new of 2 records/)).toBeVisible();
  fireEvent.click(screen.getByText(/Agent Deck activity/));
  expect(screen.getByText('Review complete')).toBeVisible();
});

it('requires an upgrade choice and can persist Agent Deck as the default', async () => {
  const value = emptyStatus();
  value.defaultProvider = 'choose';
  value.providers[1] = { ...value.providers[1], available: true };
  mocks.status.mockResolvedValue(value);
  render(<CoordinationDialog onClose={() => {}} />);

  expect(await screen.findByText(/one-time upgrade choice/)).toBeVisible();
  expect(screen.queryByText('Start a coding agent')).not.toBeInTheDocument();
  fireEvent.change(screen.getByLabelText('Coordinator'), { target: { value: 'agent-deck' } });
  fireEvent.click(screen.getByRole('button', { name: 'Make default' }));

  await waitFor(() => expect(mocks.updateConfig).toHaveBeenCalledWith({ app: { coordination: { default_provider: 'agent-deck' } } }));
  expect(await screen.findByText('Using')).toHaveTextContent('Using Agent Deck');
});

it('temporarily switches providers without rewriting the default', async () => {
  const value = emptyStatus();
  value.providers[1] = { ...value.providers[1], available: true };
  mocks.status.mockResolvedValue(value);
  render(<CoordinationDialog onClose={() => {}} />);

  await screen.findByText('Using');
  fireEvent.change(screen.getByLabelText('Coordinator'), { target: { value: 'agent-deck' } });
  fireEvent.click(screen.getByRole('button', { name: 'Use this time' }));

  expect(await screen.findByText(/default: Rig/)).toBeVisible();
  expect(mocks.updateConfig).not.toHaveBeenCalled();
});

it('suggests meaningful editable names from a folder and purpose without launching on navigation', async () => {
  render(<CoordinationDialog inline onClose={() => {}} />);
  await screen.findByText('Using');
  expect(screen.queryByRole('dialog')).toBeNull();
  fireEvent.change(screen.getByLabelText('Code folder'), { target: { value: '/work/payment-service' } });
  expect(screen.getByLabelText('Project name')).toHaveValue('payment-service');
  expect(screen.getByLabelText('Session name')).toHaveValue('codex-builder');
  fireEvent.change(screen.getByLabelText('Session purpose'), { target: { value: 'reviewer' } });
  expect(screen.getByLabelText('Session name')).toHaveValue('codex-reviewer');
  fireEvent.change(screen.getByLabelText('Coding agent'), { target: { value: 'claude-code' } });
  expect(screen.getByLabelText('Session name')).toHaveValue('claude-reviewer');
  fireEvent.change(screen.getByLabelText('Session name'), { target: { value: 'security-review' } });
  fireEvent.change(screen.getByLabelText('Code folder'), { target: { value: '/work/other-service' } });
  expect(screen.getByLabelText('Project name')).toHaveValue('other-service');
  expect(screen.getByLabelText('Session name')).toHaveValue('security-review');
  expect(mocks.create).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Start agent and open terminal' }));
  await waitFor(() => expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({ mode: 'new', projectName: 'other-service', name: 'security-review', workspace: '/work/other-service', runtime: 'claude-code' })));
});

it('avoids name collisions and adds an agent to an existing project', async () => {
  const value = emptyStatus();
  value.projects = [{ provider: 'rig', id: 'project-one', name: 'webmux', host: 'localhost', state: 'running', historical: false, seatCount: 1, runningCount: 1, contexts: ['agents'] }];
  value.seats = [{ provider: 'rig', id: 'worker-one', projectId: 'project-one', projectName: 'webmux', name: 'codex-builder', host: 'localhost', runtime: 'codex', state: 'running', openable: true, managed: true }];
  mocks.status.mockResolvedValue(value);
  render(<CoordinationDialog onClose={() => {}} />);
  await screen.findByText('Using');
  fireEvent.change(screen.getByLabelText('Code folder'), { target: { value: '/work/webmux' } });
  expect(screen.getByLabelText('Project name')).toHaveValue('webmux-2');
  fireEvent.click(screen.getByLabelText('Add an agent to an existing project'));
  expect(screen.getByLabelText('Session name')).toHaveValue('codex-builder-2');
  fireEvent.click(screen.getByRole('button', { name: 'Start agent and open terminal' }));
  await waitFor(() => expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({ mode: 'existing', projectId: 'project-one', projectName: 'webmux', context: 'agents', name: 'codex-builder-2' })));
});
