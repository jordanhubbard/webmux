import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { ServersDialog } from '@frontend/components/ServersDialog';

const mocks = vi.hoisted(() => ({ directory: vi.fn(), save: vi.fn(), remove: vi.fn(), sessions: vi.fn(), grant: vi.fn(), removeGrant: vi.fn() }));
vi.mock('@frontend/utils/api', () => ({ api: { getServerDirectory: mocks.directory, saveServer: mocks.save, deleteServer: mocks.remove, getServerSessions: mocks.sessions, saveServerGrant: mocks.grant, deleteServerGrant: mocks.removeGrant } }));
const peer = { id: 'remote', name: 'Builder', url: 'https://builder', allow_http: false, token_env: 'BUILDER_TOKEN', credential_ready: false };
beforeEach(() => {
  vi.clearAllMocks();
  mocks.directory.mockResolvedValue({ servers: [peer], grants: [] });
  mocks.save.mockResolvedValue(undefined); mocks.grant.mockResolvedValue(undefined); mocks.remove.mockResolvedValue(undefined);
});

describe('server management', () => {
  it('adds the first peer and discards its transient credential after saving', async () => {
    mocks.directory.mockResolvedValue({ servers: [], grants: [] });
    const changed = vi.fn().mockResolvedValue(undefined);
    render(<ServersDialog onClose={vi.fn()} onChanged={changed} currentUser="owner" />);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Add server' })).toBeEnabled());
    fireEvent.click(screen.getByRole('button', { name: 'Add server' }));
    fireEvent.change(screen.getByLabelText('ID'), { target: { value: 'new' } });
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'New host' } });
    fireEvent.change(screen.getByLabelText('Server URL'), { target: { value: 'https://new' } });
    fireEvent.change(screen.getByLabelText('Credential', { exact: true }), { target: { value: 'a'.repeat(40) } });
    fireEvent.click(screen.getByRole('button', { name: 'Save server' }));
    await waitFor(() => expect(mocks.save).toHaveBeenCalledWith('new', { name: 'New host', url: 'https://new', allow_http: false, token_env: '', credential: 'a'.repeat(40) }));
    await waitFor(() => expect(changed).toHaveBeenCalled());
    expect(screen.queryByDisplayValue('a'.repeat(40))).not.toBeInTheDocument();
  });
  it('repairs a missing credential and reports saved-peer connection errors', async () => {
    mocks.sessions.mockRejectedValue(new Error('Peer rejected credential'));
    render(<ServersDialog onClose={vi.fn()} onChanged={vi.fn().mockResolvedValue(undefined)} currentUser="owner" />);
    fireEvent.click(await screen.findByRole('button', { name: 'Test connection to Builder' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Peer rejected credential');
    fireEvent.click(screen.getByRole('button', { name: 'Edit Builder' }));
    expect(screen.getByLabelText('ID')).toBeDisabled();
    const credential = screen.getByLabelText('Replacement credential');
    expect(credential).toHaveAttribute('type', 'password');
    expect(credential).toHaveValue('');
    fireEvent.change(credential, { target: { value: 'b'.repeat(40) } });
    fireEvent.click(screen.getByRole('button', { name: 'Save server' }));
    await waitFor(() => expect(mocks.save).toHaveBeenCalledWith('remote', expect.objectContaining({ credential: 'b'.repeat(40), token_env: 'BUILDER_TOKEN' })));
  });
  it('creates an incoming grant with the intended local owner', async () => {
    render(<ServersDialog onClose={vi.fn()} onChanged={vi.fn().mockResolvedValue(undefined)} currentUser="owner" />);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Add incoming grant' })).toBeEnabled());
    fireEvent.click(screen.getByRole('button', { name: 'Add incoming grant' }));
    expect(screen.getByLabelText('Session owner')).toHaveValue('owner');
    fireEvent.change(screen.getByLabelText('ID'), { target: { value: 'controller' } });
    fireEvent.change(screen.getByLabelText('Credential', { exact: true }), { target: { value: 'c'.repeat(40) } });
    fireEvent.click(screen.getByRole('button', { name: 'Save grant' }));
    await waitFor(() => expect(mocks.grant).toHaveBeenCalledWith('controller', { owner: 'owner', credential: 'c'.repeat(40), token_env: '' }));
  });
});
