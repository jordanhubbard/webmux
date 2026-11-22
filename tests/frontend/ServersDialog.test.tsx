import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { ServersDialog } from '@frontend/components/ServersDialog';

const mocks = vi.hoisted(() => ({ verifyIdentity: vi.fn(), discovery: vi.fn(), directory: vi.fn(), save: vi.fn(), remove: vi.fn(), sessions: vi.fn(), grant: vi.fn(), removeGrant: vi.fn() }));
vi.mock('@frontend/utils/api', () => ({ api: { verifyServerIdentity: mocks.verifyIdentity, getDiscoveredNodes: mocks.discovery, getServerDirectory: mocks.directory, saveServer: mocks.save, deleteServer: mocks.remove, getServerSessions: mocks.sessions, saveServerGrant: mocks.grant, deleteServerGrant: mocks.removeGrant } }));
const peer = { id: 'remote', name: 'Builder', url: 'https://builder', allow_http: false, token_env: 'BUILDER_TOKEN', credential_ready: false };
beforeEach(() => {
  vi.clearAllMocks();
  mocks.verifyIdentity.mockResolvedValue(undefined);
  mocks.discovery.mockResolvedValue({ started: true, error: '', policy: { browse: true }, candidates: [] });
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
    fireEvent.change(screen.getByLabelText('Name (optional)'), { target: { value: 'New host' } });
    fireEvent.change(screen.getByLabelText('Server URL'), { target: { value: 'https://new' } });
    fireEvent.change(screen.getByLabelText('Credential', { exact: true }), { target: { value: 'a'.repeat(40) } });
    fireEvent.click(screen.getByRole('button', { name: 'Save server' }));
    await waitFor(() => expect(mocks.save).toHaveBeenCalledWith('new', { name: 'New host', url: 'https://new', allow_http: false, coordination_owner: '', token_env: '', credential: 'a'.repeat(40) }));
    await waitFor(() => expect(changed).toHaveBeenCalled());
    expect(screen.queryByDisplayValue('a'.repeat(40))).not.toBeInTheDocument();
  });
  it('repairs a missing credential and reports saved-peer connection errors', async () => {
    mocks.sessions.mockRejectedValue(new Error('Peer rejected credential'));
    render(<ServersDialog onClose={vi.fn()} onChanged={vi.fn().mockResolvedValue(undefined)} currentUser="owner" />);
    fireEvent.click(await screen.findByRole('button', { name: 'Test connection to Builder' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Peer rejected credential');
    fireEvent.click(screen.getByRole('button', { name: 'Edit Builder' }));
    expect(screen.queryByLabelText('ID')).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Server URL'), { target: { value: 'https://moved' } });
    const credential = screen.getByLabelText('Replacement credential');
    expect(credential).toHaveAttribute('type', 'password');
    expect(credential).toHaveValue('');
    fireEvent.change(credential, { target: { value: 'b'.repeat(40) } });
    fireEvent.change(screen.getByLabelText('Name (optional)'), { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save server' }));
    await waitFor(() => expect(mocks.save).toHaveBeenCalledWith('remote', expect.objectContaining({ name: 'moved', url: 'https://moved', credential: 'b'.repeat(40), token_env: 'BUILDER_TOKEN' })));
  });
  it('creates an incoming grant with the intended local owner', async () => {
    render(<ServersDialog onClose={vi.fn()} onChanged={vi.fn().mockResolvedValue(undefined)} currentUser="owner" />);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Add incoming grant' })).toBeEnabled());
    fireEvent.click(screen.getByRole('button', { name: 'Add incoming grant' }));
    expect(screen.getByLabelText('Session owner')).toHaveValue('owner');
    fireEvent.change(screen.getByLabelText('Grant name'), { target: { value: 'controller' } });
    fireEvent.change(screen.getByLabelText('Credential', { exact: true }), { target: { value: 'c'.repeat(40) } });
    fireEvent.click(screen.getByRole('button', { name: 'Save grant' }));
    await waitFor(() => expect(mocks.grant).toHaveBeenCalledWith('controller', { owner: 'owner', coordination: false, credential: 'c'.repeat(40), token_env: '' }));
  });
});

it('opens the add editor directly from the cluster shortcut', async () => {
  render(<ServersDialog initiallyAddServer onClose={vi.fn()} onChanged={vi.fn()} currentUser="owner" />);
  expect(screen.queryByLabelText('ID')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Server URL')).toHaveValue('');
  expect(screen.getByRole('button', { name: 'Save server' })).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(await screen.findByRole('button', { name: 'Edit Builder' })).toBeVisible();
});

it('removes a peer only after confirmation and refreshes the directory', async () => {
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
  const changed = vi.fn().mockResolvedValue(undefined);
  try {
    render(<ServersDialog onClose={vi.fn()} onChanged={changed} currentUser="owner" />);
    fireEvent.click(await screen.findByRole('button', { name: 'Remove Builder' }));
    expect(mocks.remove).not.toHaveBeenCalled();
    confirm.mockReturnValue(true);
    mocks.directory.mockResolvedValue({ servers: [], grants: [] });
    fireEvent.click(screen.getByRole('button', { name: 'Remove Builder' }));
    await waitFor(() => expect(changed).toHaveBeenCalledOnce());
    expect(mocks.remove).toHaveBeenCalledWith('remote');
    expect(await screen.findByText(/No server peers configured/)).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Refresh directory' }));
    await waitFor(() => expect(changed).toHaveBeenCalledTimes(2));
  } finally { confirm.mockRestore(); }
});

it.each([
  ['https://builder:8443', 'builder-8443', 'builder:8443'],
  ['https://builder', 'builder', 'builder'],
  ['https://local', 'local-2', 'local'],
  ['https://Builder:9443', 'builder-9443-2', 'builder:9443'],
  ['https://all', 'all-2', 'all'],
  ['https://_builder', 'builder', '_builder'],
  ['https://[::1]:8443', '1-8443', '[::1]:8443'],
  ['https://controller', 'controller-2', 'controller'],
  [`https://${'a'.repeat(63)}.example.net`, 'a'.repeat(56), `${'a'.repeat(63)}.example.net`],
])('derives a unique ID and default name from %s', async (url, id, name) => {
  mocks.directory.mockResolvedValue({ servers: [peer, { ...peer, id: 'builder-9443' }], grants: [{ id: 'controller', owner: 'owner' }] });
  render(<ServersDialog initiallyAddServer onClose={vi.fn()} onChanged={vi.fn()} currentUser="owner" />);
  await waitFor(() => expect(screen.getByRole('button', { name: 'Save server' })).toBeEnabled());
  fireEvent.change(screen.getByLabelText('Server URL'), { target: { value: url } });
  fireEvent.click(screen.getByRole('button', { name: 'Save server' }));
  await waitFor(() => expect(mocks.save).toHaveBeenCalledWith(id, expect.objectContaining({ name, url })));
});

it('shows untrusted discovered candidates without enrolling them or enabling HTTP', async () => {
  mocks.discovery.mockResolvedValue({ started: true, error: '', policy: { browse: true }, candidates: [
    { instance: 'WebMux-candidate', claimed_id: 'a'.repeat(32), host: 'nearby.local', address: '192.0.2.10', port: 8081, tls: false },
  ] });
  render(<ServersDialog onClose={vi.fn()} onChanged={vi.fn()} currentUser="owner" />);
  fireEvent.click(await screen.findByRole('button', { name: 'Configure connection to nearby.local' }));
  expect(mocks.save).not.toHaveBeenCalled();
  expect(mocks.grant).not.toHaveBeenCalled();
  expect(screen.getByLabelText('Server URL')).toHaveValue('http://nearby.local:8081');
  expect(screen.getByLabelText('Allow HTTP for a trusted private network or tunnel')).not.toBeChecked();
  expect(screen.getByLabelText('Credential', { exact: true })).toHaveValue('');
});

it('preserves manual configuration when network discovery is unavailable', async () => {
  mocks.discovery.mockRejectedValue(new Error('Multicast unavailable'));
  render(<ServersDialog onClose={vi.fn()} onChanged={vi.fn()} currentUser="owner" />);
  expect(await screen.findByText('Cannot load network discovery. Manual connections remain available.')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Add server' })).toBeEnabled();
  expect(screen.getByRole('button', { name: 'Edit Builder' })).toBeEnabled();
});

it('verifies the enrolled node and receiving account before showing its pin', async () => {
  const readyPeer = { ...peer, credential_ready: true };
  mocks.directory.mockResolvedValue({ servers: [readyPeer], grants: [] });
  render(<ServersDialog onClose={vi.fn()} onChanged={vi.fn()} currentUser="owner" />);
  const verify = await screen.findByRole('button', { name: 'Verify identity of Builder' });
  mocks.directory.mockResolvedValue({ servers: [{ ...readyPeer, identity: { node_id: 'a'.repeat(32), owner: 'owner', tls_key_sha256: 'b'.repeat(64) } }], grants: [] });
  fireEvent.click(verify);
  await waitFor(() => expect(mocks.verifyIdentity).toHaveBeenCalledWith('remote'));
  expect(await screen.findByText(/Verified node: a+ · Account: owner/)).toBeVisible();
  expect(mocks.save).not.toHaveBeenCalled();
});

it('keeps the original identity visible when verification fails', async () => {
  mocks.directory.mockResolvedValue({ servers: [{ ...peer, credential_ready: true, identity: { node_id: 'a'.repeat(32), owner: 'owner', tls_key_sha256: 'b'.repeat(64) } }], grants: [] });
  mocks.verifyIdentity.mockRejectedValue(new Error('Original identity no longer matches'));
  render(<ServersDialog onClose={vi.fn()} onChanged={vi.fn()} currentUser="owner" />);
  fireEvent.click(await screen.findByRole('button', { name: 'Verify identity of Builder' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Original identity no longer matches');
  expect(screen.getByText(/Verified node: a+ · Account: owner/)).toBeVisible();
  expect(mocks.save).not.toHaveBeenCalled();
});


describe('coordination sharing authorization', () => {
  it('requires a verified node before showing the local account mapping', async () => {
    render(<ServersDialog onClose={vi.fn()} onChanged={vi.fn()} currentUser="owner" />);
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Builder' }));
    expect(screen.queryByLabelText('Coordination account on this server')).not.toBeInTheDocument();
    expect(screen.getByText(/Verify the HTTPS node identity after saving/)).toBeVisible();
  });
  it('saves an explicit local-account mapping for an enrolled peer', async () => {
    mocks.directory.mockResolvedValue({ servers: [{ ...peer, identity: { node_id: 'a'.repeat(32), owner: 'remote-owner', tls_key_sha256: 'b'.repeat(64) } }], grants: [] });
    render(<ServersDialog onClose={vi.fn()} onChanged={vi.fn()} currentUser="owner" />);
    fireEvent.click(await screen.findByRole('button', { name: 'Edit Builder' }));
    expect(screen.getByLabelText('Coordination account on this server')).toHaveValue('');
    fireEvent.change(screen.getByLabelText('Coordination account on this server'), { target: { value: 'owner' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save server' }));
    await waitFor(() => expect(mocks.save).toHaveBeenCalledWith('remote', expect.objectContaining({ coordination_owner: 'owner' })));
  });
  it('keeps existing incoming grants terminal-only until sharing is selected', async () => {
    mocks.directory.mockResolvedValue({ servers: [], grants: [{ id: 'incoming', owner: 'owner', token_env: 'TOKEN', credential_ready: true }] });
    render(<ServersDialog onClose={vi.fn()} onChanged={vi.fn()} currentUser="owner" />);
    fireEvent.click(await screen.findByRole('button', { name: 'Edit grant incoming' }));
    expect(screen.getByLabelText('Allow project coordination summaries')).not.toBeChecked();
    fireEvent.click(screen.getByLabelText('Allow project coordination summaries'));
    fireEvent.click(screen.getByRole('button', { name: 'Save grant' }));
    await waitFor(() => expect(mocks.grant).toHaveBeenCalledWith('incoming', expect.objectContaining({ owner: 'owner', coordination: true })));
  });
});
