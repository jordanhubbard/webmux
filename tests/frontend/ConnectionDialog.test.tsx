import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ConnectionDialog } from '@frontend/components/ConnectionDialog';

vi.mock('@frontend/utils/api', () => ({
  api: {
    getHosts: vi.fn().mockResolvedValue([
      { id: 'h1', hostname: 'host1.example.com', port: 22, tags: ['linux'], mosh_allowed: false },
    ]),
    getKeys: vi.fn().mockResolvedValue([
      { id: 'k1', type: 'rsa', encrypted: false, description: 'Test Key' },
    ]),
  },
}));

describe('ConnectionDialog', () => {
  const onConnect = vi.fn();
  const onClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders dialog with tabs and form fields', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    await waitFor(() => {
      expect(screen.getByText('New SSH Session')).toBeDefined();
    });
    expect(screen.getByText('Saved Host')).toBeDefined();
    expect(screen.getByText('Ad-hoc')).toBeDefined();
    expect(screen.getByText('Password')).toBeDefined();
    expect(screen.getByText('Key')).toBeDefined();
    expect(screen.getByText('Connect')).toBeDefined();
  });

  it('shows error when username is empty', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.click(screen.getByText('Connect'));
    await waitFor(() => {
      expect(screen.getByText('Username is required')).toBeDefined();
    });
    expect(onConnect).not.toHaveBeenCalled();
  });

  it('shows ad-hoc hostname field when Ad-hoc tab selected', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.click(screen.getByText('Ad-hoc'));
    expect(screen.getByPlaceholderText('hostname or IP')).toBeDefined();
  });

  it('shows hostname required error for adhoc without hostname', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.click(screen.getByText('Ad-hoc'));

    const usernameInput = screen.getByPlaceholderText('user');
    fireEvent.change(usernameInput, { target: { value: 'testuser' } });
    fireEvent.click(screen.getByText('Connect'));

    await waitFor(() => {
      expect(screen.getByText('Hostname is required')).toBeDefined();
    });
  });

  it('calls onConnect with correct data for ad-hoc connection', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.click(screen.getByText('Ad-hoc'));

    fireEvent.change(screen.getByPlaceholderText('hostname or IP'), { target: { value: 'myhost.com' } });
    fireEvent.change(screen.getByPlaceholderText('user'), { target: { value: 'admin' } });
    fireEvent.change(screen.getByPlaceholderText('Remote password (not stored)'), { target: { value: 'secret' } });
    fireEvent.click(screen.getByText('Connect'));

    await waitFor(() => {
      expect(onConnect).toHaveBeenCalledWith(
        expect.objectContaining({
          username: 'admin',
          hostname: 'myhost.com',
          password: 'secret',
          transport: 'ssh',
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
    // The backdrop is the outermost div
    const backdrop = screen.getByText('New SSH Session').closest('div')!.parentElement!.parentElement!;
    fireEvent.click(backdrop);
    expect(onClose).toHaveBeenCalled();
  });

  it('shows key selector when Key auth tab selected', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.click(screen.getByText('Key'));
    await waitFor(() => {
      expect(screen.getByText(/Default key/)).toBeDefined();
      expect(screen.getByText(/Test Key/)).toBeDefined();
    });
  });

  it('passes key_id in request when key auth selected', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    fireEvent.click(screen.getByText('Ad-hoc'));
    fireEvent.change(screen.getByPlaceholderText('hostname or IP'), { target: { value: 'myhost.com' } });
    fireEvent.change(screen.getByPlaceholderText('user'), { target: { value: 'admin' } });
    fireEvent.click(screen.getByText('Key'));

    await waitFor(() => {
      expect(screen.getByText(/Test Key/)).toBeDefined();
    });

    // Select the key
    const select = screen.getByDisplayValue(/Default key/);
    fireEvent.change(select, { target: { value: 'k1' } });
    fireEvent.click(screen.getByText('Connect'));

    await waitFor(() => {
      expect(onConnect).toHaveBeenCalledWith(
        expect.objectContaining({
          key_id: 'k1',
        })
      );
    });
  });

  it('passes suggestedRow and suggestedCol', async () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} suggestedRow={2} suggestedCol={3} />);
    fireEvent.click(screen.getByText('Ad-hoc'));
    fireEvent.change(screen.getByPlaceholderText('hostname or IP'), { target: { value: 'h' } });
    fireEvent.change(screen.getByPlaceholderText('user'), { target: { value: 'u' } });
    fireEvent.click(screen.getByText('Connect'));

    await waitFor(() => {
      expect(onConnect).toHaveBeenCalledWith(
        expect.objectContaining({ row: 2, col: 3 })
      );
    });
  });

  it('shows mosh transport option', () => {
    render(<ConnectionDialog onConnect={onConnect} onClose={onClose} />);
    expect(screen.getByText(/Mosh/)).toBeDefined();
  });
});
