import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ConnectionDialog } from '@frontend/components/ConnectionDialog';

const mockApi = vi.hoisted(() => ({
  getHosts: vi.fn().mockResolvedValue([
    { id: 'h1', hostname: 'host1.example.com', port: 22, tags: ['linux'], mosh_allowed: false },
  ]),
  getKeys: vi.fn().mockResolvedValue([
    { id: 'k1', type: 'rsa', encrypted: false, description: 'Test Key' },
  ]),
  createHost: vi.fn().mockResolvedValue({ id: 'h-new', hostname: 'new.example.com', port: 22, tags: [], mosh_allowed: false }),
  deleteHost: vi.fn().mockResolvedValue(undefined),
}));

vi.mock('@frontend/utils/api', () => ({
  api: mockApi,
}));

describe('ConnectionDialog', () => {
  const onConnect = vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    onConnect.mockResolvedValue(undefined);
    mockApi.getHosts.mockResolvedValue([
      { id: 'h1', hostname: 'host1.example.com', port: 22, tags: ['linux'], mosh_allowed: false },
    ]);
    mockApi.getKeys.mockResolvedValue([
      { id: 'k1', type: 'rsa', encrypted: false, description: 'Test Key' },
    ]);
    mockApi.createHost.mockResolvedValue({ id: 'h-new', hostname: 'new.example.com', port: 22, tags: [], mosh_allowed: false });
    mockApi.deleteHost.mockResolvedValue(undefined);
  });

  it('renders dialog with hostname and username fields', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    await waitFor(() => {
      expect(screen.getByText('Connect to Host')).toBeDefined();
    });
    expect(screen.getByPlaceholderText('hostname or IP')).toBeDefined();
    expect(screen.getByPlaceholderText('user')).toBeDefined();
    expect(screen.getByText('Connect')).toBeDefined();
  });

  it('shows error when hostname is empty', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.change(screen.getByPlaceholderText('user'), { target: { value: 'admin' } });
    fireEvent.click(screen.getByText('Connect'));
    await waitFor(() => {
      expect(screen.getByText('Hostname is required')).toBeDefined();
    });
    expect(onConnect).not.toHaveBeenCalled();
  });

  it('shows error when username is empty', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.change(screen.getByPlaceholderText('hostname or IP'), { target: { value: 'myhost' } });
    fireEvent.click(screen.getByText('Connect'));
    await waitFor(() => {
      expect(screen.getByText('Username is required')).toBeDefined();
    });
    expect(onConnect).not.toHaveBeenCalled();
  });

  it('calls onConnect with hostname and username', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.change(screen.getByPlaceholderText('hostname or IP'), { target: { value: 'sparky.local' } });
    fireEvent.change(screen.getByPlaceholderText('user'), { target: { value: 'admin' } });
    fireEvent.click(screen.getByText('Connect'));

    await waitFor(() => {
      expect(onConnect).toHaveBeenCalledWith(
        expect.objectContaining({
          username: 'admin',
          hostname: 'sparky.local',
          port: 22,
          transport: 'ssh',
        })
      );
    });
  });

  it('sends no credentials in default agent mode', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.change(screen.getByPlaceholderText('hostname or IP'), { target: { value: 'sparky.local' } });
    fireEvent.change(screen.getByPlaceholderText('user'), { target: { value: 'admin' } });
    fireEvent.click(screen.getByText('Connect'));

    await waitFor(() => {
      expect(onConnect).toHaveBeenCalled();
      const req = onConnect.mock.calls[0][0];
      expect(req.password).toBeUndefined();
      expect(req.key_id).toBeUndefined();
    });
  });

  it('sends password when password auth selected', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.change(screen.getByPlaceholderText('hostname or IP'), { target: { value: 'myhost.com' } });
    fireEvent.change(screen.getByPlaceholderText('user'), { target: { value: 'admin' } });

    fireEvent.click(screen.getByText(/Advanced options/));
    fireEvent.click(screen.getByText('Password'));
    fireEvent.change(screen.getByPlaceholderText('Remote password (requires sshpass)'), { target: { value: 'secret' } });
    fireEvent.click(screen.getByText('Connect'));

    await waitFor(() => {
      expect(onConnect).toHaveBeenCalledWith(
        expect.objectContaining({
          username: 'admin',
          hostname: 'myhost.com',
          password: 'secret',
        })
      );
    });
  });

  it('calls onClose when cancel clicked', () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.click(screen.getByText('Cancel'));
    expect(onClose).toHaveBeenCalled();
  });

  it('calls onClose when backdrop clicked', () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    const backdrop = screen.getByText('Connect to Host').closest('div')!.parentElement!.parentElement!;
    fireEvent.click(backdrop);
    expect(onClose).toHaveBeenCalled();
  });

  it('shows key selector in advanced options', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.click(screen.getByText(/Advanced options/));
    fireEvent.click(screen.getByText('Key'));
    await waitFor(() => {
      expect(screen.getByText(/Default key/)).toBeDefined();
      expect(screen.getByText(/Test Key/)).toBeDefined();
    });
  });

  it('passes key_id when key auth selected', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.change(screen.getByPlaceholderText('hostname or IP'), { target: { value: 'myhost.com' } });
    fireEvent.change(screen.getByPlaceholderText('user'), { target: { value: 'admin' } });

    fireEvent.click(screen.getByText(/Advanced options/));
    fireEvent.click(screen.getByText('Key'));

    await waitFor(() => {
      expect(screen.getByText(/Test Key/)).toBeDefined();
    });

    const select = screen.getByDisplayValue(/Default key/);
    fireEvent.change(select, { target: { value: 'k1' } });
    fireEvent.click(screen.getByText('Connect'));

    await waitFor(() => {
      expect(onConnect).toHaveBeenCalledWith(
        expect.objectContaining({ key_id: 'k1' })
      );
    });
  });

  it('passes suggestedRow and suggestedCol', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} suggestedRow={2} suggestedCol={3} />);
    fireEvent.change(screen.getByPlaceholderText('hostname or IP'), { target: { value: 'h' } });
    fireEvent.change(screen.getByPlaceholderText('user'), { target: { value: 'u' } });
    fireEvent.click(screen.getByText('Connect'));

    await waitFor(() => {
      expect(onConnect).toHaveBeenCalledWith(
        expect.objectContaining({ row: 2, col: 3 })
      );
    });
  });

  it('shows saved hosts as clickable cards', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    await waitFor(() => {
      expect(screen.getByText('Saved Hosts')).toBeDefined();
      expect(screen.getByText('host1.example.com')).toBeDefined();
    });
  });

  it('quick-connects when clicking a saved host card', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    await waitFor(() => {
      expect(screen.getByText('host1.example.com')).toBeDefined();
    });

    fireEvent.change(screen.getByPlaceholderText('user'), { target: { value: 'testuser' } });
    fireEvent.click(screen.getByText('host1.example.com'));

    await waitFor(() => {
      expect(onConnect).toHaveBeenCalledWith(
        expect.objectContaining({
          host_id: 'h1',
          hostname: 'host1.example.com',
          username: 'testuser',
        })
      );
    });
  });

  it('shows error when clicking saved host without username', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    await waitFor(() => {
      expect(screen.getByText('host1.example.com')).toBeDefined();
    });

    fireEvent.click(screen.getByText('host1.example.com'));

    await waitFor(() => {
      expect(screen.getByText(/Enter a username first/)).toBeDefined();
    });
    expect(onConnect).not.toHaveBeenCalled();
  });

  it('deletes a saved host when remove button clicked', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    await waitFor(() => {
      expect(screen.getByText('host1.example.com')).toBeDefined();
    });

    const removeBtn = screen.getByTitle('Remove saved host');
    fireEvent.click(removeBtn);

    await waitFor(() => {
      expect(mockApi.deleteHost).toHaveBeenCalledWith('h1');
    });
  });

  it('saves and connects with Save & Connect button', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.change(screen.getByPlaceholderText('hostname or IP'), { target: { value: 'new.example.com' } });
    fireEvent.change(screen.getByPlaceholderText('user'), { target: { value: 'admin' } });

    await waitFor(() => {
      expect(screen.getByText('Save & Connect')).toBeDefined();
    });

    fireEvent.click(screen.getByText('Save & Connect'));

    await waitFor(() => {
      expect(mockApi.createHost).toHaveBeenCalledWith(
        expect.objectContaining({
          hostname: 'new.example.com',
          port: 22,
        })
      );
      expect(onConnect).toHaveBeenCalledWith(
        expect.objectContaining({
          host_id: 'h-new',
          hostname: 'new.example.com',
          username: 'admin',
        })
      );
    });
  });

  it('hides Save & Connect when hostname matches a saved host', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    await waitFor(() => {
      expect(screen.getByText('host1.example.com')).toBeDefined();
    });

    fireEvent.change(screen.getByPlaceholderText('hostname or IP'), { target: { value: 'host1.example.com' } });
    expect(screen.queryByText('Save & Connect')).toBeNull();
  });

  it('shows mosh transport in advanced options', () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.click(screen.getByText(/Advanced options/));
    expect(screen.getByText('Mosh')).toBeDefined();
  });

  it('surfaces API errors in the dialog', async () => {
    onConnect.mockRejectedValueOnce(new Error('Connection refused'));
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.change(screen.getByPlaceholderText('hostname or IP'), { target: { value: 'bad.host' } });
    fireEvent.change(screen.getByPlaceholderText('user'), { target: { value: 'user' } });
    fireEvent.click(screen.getByText('Connect'));

    await waitFor(() => {
      expect(screen.getByText('Connection refused')).toBeDefined();
    });
  });
});
