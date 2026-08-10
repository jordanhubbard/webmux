import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

const mockApi = vi.hoisted(() => ({
  getUsers: vi.fn(),
  register: vi.fn(),
  deleteUser: vi.fn(),
}));

vi.mock('@frontend/utils/api', () => ({ api: mockApi }));

import { UsersDialog } from '@frontend/components/UsersDialog';

describe('UsersDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApi.getUsers.mockResolvedValue([
      { username: 'owner', admin: true },
      { username: 'guest', admin: false },
    ]);
    mockApi.register.mockResolvedValue({ username: 'new', admin: false });
    mockApi.deleteUser.mockResolvedValue(undefined);
  });

  it('lists users with admin badges', async () => {
    render(<UsersDialog currentUser="owner" onClose={vi.fn()} />);
    await waitFor(() => expect(screen.getByText('owner')).toBeDefined());
    expect(screen.getByText('guest')).toBeDefined();
    expect(screen.getByText('admin')).toBeDefined();
  });

  it('disables removing yourself', async () => {
    render(<UsersDialog currentUser="owner" onClose={vi.fn()} />);
    await waitFor(() => expect(screen.getByText('owner')).toBeDefined());
    // The owner row shows "This is you" instead of a Remove button.
    expect(screen.getByText('This is you')).toBeDefined();
  });

  it('removes a guest after confirmation', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    render(<UsersDialog currentUser="owner" onClose={vi.fn()} />);
    await waitFor(() => expect(screen.getByText('guest')).toBeDefined());
    fireEvent.click(screen.getByTitle('Remove guest'));
    await waitFor(() => expect(mockApi.deleteUser).toHaveBeenCalledWith('guest'));
  });

  it('does not remove when confirmation is declined', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false);
    render(<UsersDialog currentUser="owner" onClose={vi.fn()} />);
    await waitFor(() => expect(screen.getByText('guest')).toBeDefined());
    fireEvent.click(screen.getByTitle('Remove guest'));
    expect(mockApi.deleteUser).not.toHaveBeenCalled();
  });

  it('validates the add-account form', async () => {
    render(<UsersDialog currentUser="owner" onClose={vi.fn()} />);
    await waitFor(() => expect(screen.getByText('owner')).toBeDefined());
    fireEvent.change(screen.getByPlaceholderText('username'), { target: { value: 'x' } });
    fireEvent.click(screen.getByText('Add Account'));
    await waitFor(() => expect(screen.getByText(/at least 2 characters/i)).toBeDefined());
    expect(mockApi.register).not.toHaveBeenCalled();
  });

  it('adds an account and reloads the list', async () => {
    render(<UsersDialog currentUser="owner" onClose={vi.fn()} />);
    await waitFor(() => expect(screen.getByText('owner')).toBeDefined());
    fireEvent.change(screen.getByPlaceholderText('username'), { target: { value: 'newbie' } });
    fireEvent.change(screen.getByPlaceholderText('password'), { target: { value: 'secret' } });
    fireEvent.change(screen.getByPlaceholderText('confirm'), { target: { value: 'secret' } });
    fireEvent.click(screen.getByText('Add Account'));
    await waitFor(() => expect(mockApi.register).toHaveBeenCalledWith('newbie', 'secret', false));
    expect(mockApi.getUsers).toHaveBeenCalledTimes(2);
  });
});
