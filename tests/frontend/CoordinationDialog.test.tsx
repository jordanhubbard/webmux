import { beforeEach, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
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
  defaultLaunchProvider: 'rig', defaultView: 'rig', enabledProviders: ['agent-deck', 'rig'],
  providers: [
    { id: 'rig', label: 'Rig', available: true, enabled: true, capabilities: ['projects', 'topology', 'launch', 'attach'], contract: 'openrig-cli/0.6.14' },
    { id: 'agent-deck', label: 'Agent Deck', available: false, enabled: true, message: 'Install Agent Deck', capabilities: ['sessions'], contract: 'agent-deck-cli/v1' },
  ],
  projects: [], seats: [], messages: [], errors: {},
});

beforeEach(() => {
  vi.clearAllMocks();
  mocks.status.mockResolvedValue(emptyStatus());
  mocks.create.mockResolvedValue({ target: 'new-seat', session: { id: 'viewer' } });
  mocks.attach.mockResolvedValue({ id: 'viewer' });
  mocks.browse.mockResolvedValue({ host: 'localhost', path: '/work/project', parent: '/work', repository: true, entries: [] });
  mocks.updateConfig.mockImplementation(async ({ app }) => {
    const previous = await mocks.status();
    const patch = app.coordination;
    mocks.status.mockResolvedValue({ ...previous, ...(patch.enabled_providers ? { enabledProviders: patch.enabled_providers } : {}), ...(patch.default_launch_provider ? { defaultLaunchProvider: patch.default_launch_provider } : {}), ...(patch.default_view ? { defaultView: patch.default_view } : {}) });
    return { app };
  });
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
  mocks.status.mockResolvedValue({ ...emptyStatus(), defaultLaunchProvider: 'agent-deck', defaultView: 'agent-deck' });
  render(<CoordinationDialog onClose={() => {}} />);
  await waitFor(() => expect(screen.getByLabelText('Launch owner')).toHaveValue('agent-deck'));
  expect(screen.queryByRole('link', { name: /Open Rig text dashboard/ })).toBeNull();
});

it('can disable Rig without disabling Agent Deck or stopping sessions', async () => {
  render(<CoordinationDialog onClose={() => {}} />);
  await waitFor(() => expect(screen.getByLabelText('Enable Rig')).toBeChecked());
  fireEvent.click(screen.getByLabelText('Enable Rig'));
  await waitFor(() => expect(mocks.updateConfig).toHaveBeenCalledWith({ app: { coordination: { enabled_providers: ['agent-deck'], default_launch_provider: 'webmux' } } }));
  expect(mocks.create).not.toHaveBeenCalled();
  expect(mocks.attach).not.toHaveBeenCalled();
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
  value.providers[1] = { id: 'agent-deck', label: 'Agent Deck', available: true, enabled: true, capabilities: ['sessions', 'remote', 'native-session-id'], contract: 'agent-deck-cli/v1' };
  value.defaultLaunchProvider = 'agent-deck'; value.defaultView = 'agent-deck';
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
  value.providers[1] = { id: 'agent-deck', label: 'Agent Deck', available: true, enabled: true, capabilities: ['sessions', 'remote', 'parent-child', 'send', 'queue', 'send-status', 'completion-inbox', 'remote-drain'], contract: 'agent-deck-cli/v1' };
  value.defaultLaunchProvider = 'agent-deck'; value.defaultView = 'agent-deck';
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

it('persists a launch default independently from the displayed inventory', async () => {
  render(<CoordinationDialog onClose={() => {}} />);
  await waitFor(() => expect(screen.getByLabelText('Launch owner')).toHaveValue('rig'));
  fireEvent.change(screen.getByLabelText('Launch owner'), { target: { value: 'agent-deck' } });
  fireEvent.click(screen.getByRole('button', { name: 'Make default launch owner' }));
  await waitFor(() => expect(mocks.updateConfig).toHaveBeenCalledWith({ app: { coordination: { default_launch_provider: 'agent-deck' } } }));
  expect(screen.getByLabelText('Show sessions')).toHaveValue('rig');
  expect(screen.getByLabelText('Enable Rig')).toBeChecked();
});

it('temporarily switches providers without rewriting the default', async () => {
  const value = emptyStatus();
  value.providers[1] = { ...value.providers[1], available: true };
  mocks.status.mockResolvedValue(value);
  render(<CoordinationDialog onClose={() => {}} />);

  await waitFor(() => expect(screen.getByLabelText('Launch owner')).toHaveValue('rig'));
  fireEvent.change(screen.getByLabelText('Launch owner'), { target: { value: 'agent-deck' } });

  expect(await screen.findByText(/Default launch owner: Rig/)).toBeVisible();
  expect(mocks.updateConfig).not.toHaveBeenCalled();
});

it('suggests meaningful editable names from a folder and purpose without launching on navigation', async () => {
  render(<CoordinationDialog inline onClose={() => {}} />);
  await waitFor(() => expect(screen.getByLabelText('Launch owner')).toHaveValue('rig'));
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
  await waitFor(() => expect(screen.getByLabelText('Launch owner')).toHaveValue('rig'));
  fireEvent.change(screen.getByLabelText('Code folder'), { target: { value: '/work/webmux' } });
  expect(screen.getByLabelText('Project name')).toHaveValue('webmux-2');
  fireEvent.click(screen.getByLabelText('Add an agent to an existing project'));
  expect(screen.getByLabelText('Session name')).toHaveValue('codex-builder-2');
  fireEvent.click(screen.getByRole('button', { name: 'Start agent and open terminal' }));
  await waitFor(() => expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({ mode: 'existing', projectId: 'project-one', projectName: 'webmux', context: 'agents', name: 'codex-builder-2' })));
});


it('uses built-in WebMux without offering external launches or a guarded queue', async () => {
  mocks.status.mockResolvedValue({ ...emptyStatus(), defaultLaunchProvider: 'webmux', defaultView: 'all', enabledProviders: [], providers: [
    { id: 'webmux', label: 'WebMux', available: true, enabled: true, capabilities: ['terminals', 'connected-servers', 'direct-messaging'], contract: 'webmux-server-directory/v1' },
    ...emptyStatus().providers,
  ] });
  const close = vi.fn();
  render(<CoordinationDialog onClose={close} />);
  expect(await screen.findByRole('region', { name: 'Built-in WebMux coordination' })).toHaveTextContent('No external coordinator is required');
  expect(screen.getByLabelText('Launch owner')).toHaveValue('webmux');
  expect(screen.queryByRole('button', { name: 'Start agent and open terminal' })).toBeNull();
  expect(screen.queryByLabelText('Queue safely until the target can accept input')).toBeNull();
  await waitFor(() => expect(mocks.status).toHaveBeenCalledWith());
  fireEvent.click(screen.getByRole('button', { name: 'Open terminals and servers' }));
  expect(close).toHaveBeenCalledOnce();
  expect(mocks.create).not.toHaveBeenCalled();
  expect(mocks.send).not.toHaveBeenCalled();
});

it('changes the inventory view without changing enabled integrations or launch owner', async () => {
  render(<CoordinationDialog onClose={() => {}} />);
  await waitFor(() => expect(screen.getByLabelText('Launch owner')).toHaveValue('rig'));
  fireEvent.change(screen.getByLabelText('Show sessions'), { target: { value: 'agent-deck' } });
  expect(screen.getByLabelText('Launch owner')).toHaveValue('rig');
  expect(screen.getByLabelText('Enable Rig')).toBeChecked();
  expect(screen.getByLabelText('Enable Agent Deck')).toBeChecked();
  expect(mocks.updateConfig).not.toHaveBeenCalled();
  expect(mocks.create).not.toHaveBeenCalled();
});

it('shows all owners together and routes colliding IDs to the original provider', async () => {
  const value = emptyStatus();
  value.defaultLaunchProvider = 'webmux'; value.defaultView = 'all';
  value.providers = [{ id: 'webmux', label: 'WebMux', enabled: true, available: true, capabilities: [], contract: 'native' }, ...value.providers.map(provider => ({ ...provider, available: true }))];
  value.projects = ['webmux', 'rig', 'agent-deck'].map(provider => ({ provider, id: 'shared', name: `${provider} project`, host: 'localhost', state: 'observed', historical: false, seatCount: 1, runningCount: 1 }));
  value.seats = value.projects.map(project => ({ provider: project.provider, id: 'same-id', projectId: 'shared', projectName: project.name, name: `${project.provider} worker`, host: 'localhost', runtime: 'terminal', state: 'running', openable: true, managed: project.provider !== 'webmux' }));
  mocks.status.mockResolvedValue(value);
  render(<CoordinationDialog onClose={() => {}} />);
  const deck = await screen.findByRole('article', { name: 'agent-deck worker in agent-deck project' });
  const rig = screen.getByRole('article', { name: 'rig worker in rig project' });
  const native = screen.getByRole('article', { name: 'webmux worker in webmux project' });
  expect(within(rig).getByText('Rig-managed')).toBeVisible();
  expect(within(deck).getByText('Agent Deck-managed')).toBeVisible();
  fireEvent.click(within(deck).getByRole('button', { name: 'Send instruction' }));
  expect(within(rig).queryByRole('textbox')).toBeNull();
  expect(within(native).queryByRole('textbox')).toBeNull();
  fireEvent.change(within(deck).getByLabelText('Instruction for agent-deck worker'), { target: { value: 'review the change' } });
  fireEvent.click(within(deck).getByRole('button', { name: 'Queue instruction' }));
  await waitFor(() => expect(mocks.send).toHaveBeenCalledWith('agent-deck', 'same-id', 'review the change', true));
  await waitFor(() => expect(within(deck).getByRole('status')).toHaveTextContent('queued'));
  expect(within(rig).queryByRole('status')).toBeNull();
  fireEvent.click(within(rig).getByRole('button', { name: 'Open session' }));
  await waitFor(() => expect(mocks.attach).toHaveBeenCalledWith('rig', 'same-id', undefined));
  fireEvent.change(screen.getByLabelText('Show sessions'), { target: { value: 'rig' } });
  expect(screen.queryByRole('article', { name: 'agent-deck worker in agent-deck project' })).toBeNull();
  expect(screen.getByLabelText('Launch owner')).toHaveValue('webmux');
  expect(mocks.updateConfig).not.toHaveBeenCalled();
});

it('enables both integrations without changing the view or native launch default', async () => {
  mocks.status.mockResolvedValue({ ...emptyStatus(), defaultLaunchProvider: 'webmux', defaultView: 'all', enabledProviders: [] });
  render(<CoordinationDialog onClose={() => {}} />);
  await waitFor(() => expect(mocks.status).toHaveBeenCalled());
  fireEvent.click(screen.getByLabelText('Enable Agent Deck'));
  await waitFor(() => expect(screen.getByLabelText('Enable Agent Deck')).toBeChecked());
  await waitFor(() => expect(screen.getByLabelText('Enable Rig')).not.toBeDisabled());
  fireEvent.click(screen.getByLabelText('Enable Rig'));
  await waitFor(() => expect(screen.getByLabelText('Enable Rig')).toBeChecked());
  expect(screen.getByLabelText('Enable Agent Deck')).toBeChecked();
  expect(screen.getByLabelText('Show sessions')).toHaveValue('all');
  expect(screen.getByLabelText('Launch owner')).toHaveValue('webmux');
  fireEvent.change(screen.getByLabelText('Show sessions'), { target: { value: 'rig' } });
  fireEvent.click(screen.getByRole('button', { name: 'Make default view' }));
  await waitFor(() => expect(mocks.updateConfig).toHaveBeenCalledWith({ app: { coordination: { default_view: 'rig' } } }));
  expect(screen.getByLabelText('Enable Agent Deck')).toBeChecked();
});

it('keeps the owning target unchanged across repeated delivery polls', async () => {
  vi.useFakeTimers();
  try {
    const value = emptyStatus();
    value.defaultLaunchProvider = 'agent-deck'; value.defaultView = 'all';
    value.providers[1].available = true;
    value.projects = [{ provider: 'agent-deck', id: 'project', name: 'project', host: 'localhost', state: 'active', historical: false, seatCount: 1, runningCount: 1 }];
    value.seats = [{ provider: 'agent-deck', id: 'local:worker', projectId: 'project', projectName: 'project', name: 'worker', host: 'localhost', runtime: 'codex', state: 'running', openable: true, managed: true }];
    mocks.status.mockResolvedValue(value);
    mocks.send.mockResolvedValue({ target: 'local:worker', sendId: 'pending', state: 'queued', queued: true, submitted: false });
    mocks.sendStatus.mockResolvedValue({ state: 'queued', submitted: false });
    render(<CoordinationDialog onClose={() => {}} />);
    await act(async () => {});
    fireEvent.click(screen.getByRole('button', { name: 'Send instruction' }));
    fireEvent.change(screen.getByLabelText('Instruction for worker'), { target: { value: 'review' } });
    fireEvent.click(screen.getByRole('button', { name: 'Queue instruction' }));
    await act(async () => {});
    await act(async () => { await vi.advanceTimersByTimeAsync(2500); });
    await act(async () => { await vi.advanceTimersByTimeAsync(2500); });
    expect(mocks.sendStatus).toHaveBeenCalledTimes(2);
    for (const call of mocks.sendStatus.mock.calls) expect(call).toEqual(['agent-deck', 'local:worker', 'pending']);
  } finally { vi.useRealTimers(); }
});
