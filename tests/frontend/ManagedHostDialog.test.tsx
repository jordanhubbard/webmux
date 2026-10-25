import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { ManagedHostDialog } from '@frontend/components/ManagedHostDialog';
import type { Session } from '@frontend/types';

const mockApi = vi.hoisted(() => ({
  createHost: vi.fn(),
  associateSessionHost: vi.fn(),
  bindClient: vi.fn(),
  refreshClient: vi.fn(),
  unbindClient: vi.fn(),
  revokeClient: vi.fn(),
}));
vi.mock('@frontend/utils/api', () => ({ api: mockApi }));

const session: Session = {
  id: 's1', kind: 'terminal', owner: 'owner', transport: 'ssh', host_id: '', hostname: 'worker.example',
  username: 'dev', key_id: '', port: 22, cols: 80, rows: 24, row: 0, col: 0, state: 'connected',
  created_at: '', updated_at: '', title: 'dev@worker', persistent: true, minimized: false,
};

describe('ManagedHostDialog', () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it('saves an ad hoc SSH destination and links the printed client code', async () => {
    mockApi.createHost.mockResolvedValue({ id: 'host-1' });
    mockApi.associateSessionHost.mockResolvedValue({ ...session, host_id: 'host-1' });
    mockApi.bindClient.mockResolvedValue({ client_id: 'client-1', host_id: 'host-1' });
    const changed = vi.fn();
    const updated = vi.fn();
    const first = render(<ManagedHostDialog session={session} onClose={vi.fn()} onChanged={changed} onSessionUpdated={updated} />);
    fireEvent.click(screen.getByText('Save this host'));
    await waitFor(() => expect(updated).toHaveBeenCalledWith(expect.objectContaining({ host_id: 'host-1' })));
    expect(mockApi.createHost).toHaveBeenCalledWith(expect.objectContaining({ hostname: 'worker.example', username: 'dev', port: 22 }));
    first.rerender(<ManagedHostDialog session={{ ...session, host_id: 'host-1' }} onClose={vi.fn()} onChanged={changed} onSessionUpdated={updated} />);
    fireEvent.change(screen.getByLabelText('WebMux URL reachable from this host'), { target: { value: 'https://webmux.example:8443' } });
    expect(screen.getByText(/webmux-client --server https:\/\/webmux.example:8443 --background/)).toBeDefined();
    fireEvent.change(screen.getByLabelText('Pairing code'), { target: { value: 'abcdefghijklmnop' } });
    fireEvent.click(screen.getByText('Link client'));
    await waitFor(() => expect(mockApi.bindClient).toHaveBeenCalledWith('host-1', 'ABCDEFGHIJKLMNOP'));
    expect(changed).toHaveBeenCalled();
  });

  it('shows connected host facts and requests a refresh over the client channel', async () => {
    mockApi.refreshClient.mockResolvedValue({ hostname: 'worker', os: 'linux', arch: 'amd64', cpus: 8 });
    const changed = vi.fn();
    render(<ManagedHostDialog session={{ ...session, host_id: 'host-1' }} client={{ id: 'client-1', owner: 'owner', label: 'worker', host_id: 'host-1', online: true, snapshot: { hostname: 'worker', os: 'linux', arch: 'amd64', cpus: 8, at: '' } }} onClose={vi.fn()} onChanged={changed} onSessionUpdated={vi.fn()} />);
    expect(screen.getByText('linux / amd64')).toBeDefined();
    fireEvent.click(screen.getByText('Refresh host facts'));
    await waitFor(() => expect(mockApi.refreshClient).toHaveBeenCalledWith('client-1'));
    expect(changed).toHaveBeenCalled();
  });
});
