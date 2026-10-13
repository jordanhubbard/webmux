import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { fireEvent, render, screen, waitFor, cleanup } from '@testing-library/react';
import { SessionManagerWorkspace } from '@frontend/components/SessionManagerWorkspace';

const api = vi.hoisted(() => ({ sessionManager: vi.fn() }));
vi.mock('@frontend/utils/api', () => ({ api }));
const session = { id: 'thread-one', title: 'Fixture work', cwd: '/fixture', state: 'idle', summary: 'Recent result', summarySource: 'recent-items', observedAt: 1000, attachments: [], capabilities: { permissions: true } };
let requests: unknown[];
beforeEach(() => {
  requests = [];
  api.sessionManager.mockReset();
  api.sessionManager.mockImplementation(async (operation: string) => ({ version: 1, hostId: 'host', providerId: 'codex-local', generation: 'epoch', result:
    operation === 'list' ? { sessions: [session], nextCursor: null, coverage: { complete: true, message: 'Fixture provider' } } :
    operation === 'read' ? { session, items: [{ id: 'i', type: 'agentMessage', text: 'Work completed' }], requests, turnId: null, observedAt: 1000 } :
    operation === 'permissions.read' || operation === 'permissions.update' ? { revision: 'revision', effective: { approval: 'never', sandbox: { type: 'workspaceWrite', networkAccess: true, writableRoots: ['/custom'] }, profile: 'custom' }, appliesTo: 'subsequent-turns' } : {}
  }));
});
afterEach(cleanup);
async function connect() { render(<SessionManagerWorkspace />); fireEvent.click(await screen.findByRole('button', { name: /Fixture work/ })); await screen.findByLabelText('Message to session'); }

describe('session-manager workspace', () => {
  it('attaches exact identities and detach leaves work running', async () => {
    await connect();
    expect(api.sessionManager).toHaveBeenCalledWith('attach', { sessionId: 'thread-one' }, expect.any(String));
    fireEvent.click(screen.getByRole('button', { name: /Disconnect/ }));
    await waitFor(() => expect(api.sessionManager).toHaveBeenCalledWith('detach', { sessionId: 'thread-one' }, expect.any(String)));
    expect(api.sessionManager.mock.calls.some(c => c[0] === 'stop')).toBe(false);
  });
  it('preserves uncertain text and requires successful relevant refresh plus explicit acknowledgment', async () => {
    await connect(); fireEvent.change(screen.getByLabelText('Message to session'), { target: { value: 'Keep this draft' } });
    const original = api.sessionManager.getMockImplementation()!;
    api.sessionManager.mockImplementation((operation: string, ...args: unknown[]) => operation === 'send' ? Promise.reject(new Error('response lost')) : original(operation, ...args));
    fireEvent.click(screen.getByRole('button', { name: 'Send' })); await screen.findByRole('alert');
    expect(screen.getByLabelText('Message to session')).toHaveValue('Keep this draft');
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled();
    expect(screen.getByRole('button', { name: /I have reviewed/ })).toBeDisabled();
    api.sessionManager.mockImplementation((operation: string, ...args: unknown[]) => operation === 'list' ? Promise.reject(new Error('offline')) : original(operation, ...args));
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' })); await screen.findByText('offline');
    expect(screen.getByRole('button', { name: /I have reviewed/ })).toBeDisabled();
    api.sessionManager.mockImplementation(original); fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(screen.getByRole('button', { name: /I have reviewed/ })).toBeEnabled());
    expect(screen.getByLabelText('Message to session')).toHaveValue('Keep this draft');
    fireEvent.click(screen.getByRole('button', { name: /I have reviewed/ }));
    expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled();
    expect(api.sessionManager.mock.calls.filter(c => c[0] === 'send')).toHaveLength(1);
  });
  it('shows approval details and only the decisions offered by the runtime', async () => {
    requests = [{ id: 1, method: 'item/commandExecution/requestApproval', params: { command: 'echo fixture', reason: 'fixture reason', availableDecisions: ['decline'] } }];
    await connect(); expect(screen.getByText(/echo fixture/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Approve once' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Decline' })).toBeEnabled();
  });
  it('never replaces a custom policy without deliberate preset selection', async () => {
    await connect(); fireEvent.click(screen.getByRole('button', { name: 'Inspect permissions' }));
    const apply = await screen.findByRole('button', { name: 'Apply to this session' });
    expect(apply).toBeDisabled(); expect(screen.getByText(/\/custom/)).toBeInTheDocument();
    fireEvent.change(screen.getAllByLabelText('Replacement access preset')[0], { target: { value: 'read-only' } });
    expect(apply).toBeEnabled();
  });
  it('invalidates attachments on manager restart without sending a mutation', async () => {
    await connect();
    fireEvent.change(screen.getByLabelText('Message to session'), { target: { value: 'Draft across restart' } });
    const original = api.sessionManager.getMockImplementation()!;
    api.sessionManager.mockImplementation(async (...args: unknown[]) => ({ ...await original(...args), generation: 'restarted' }));
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(screen.queryByLabelText('Message to session')).not.toBeInTheDocument());
    expect(screen.getByRole('button', { name: /I have reviewed/ })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(screen.getByRole('button', { name: /I have reviewed/ })).toBeEnabled());
    fireEvent.click(await screen.findByRole('button', { name: /Fixture work/ }));
    expect(await screen.findByLabelText('Message to session')).toHaveValue('Draft across restart');
    expect(api.sessionManager.mock.calls.some(c => c[0] === 'send')).toBe(false);
  });

});
