import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { LoginPage } from '@frontend/components/LoginPage';
import type { AuthState } from '@frontend/hooks/useAuth';

function makeAuth(overrides: Partial<AuthState> = {}): AuthState {
  return {
    isAuthenticated: false,
    isLoading: false,
    authStatus: { mode: 'local', bootstrap_required: false },
    error: null,
    login: vi.fn().mockResolvedValue(undefined),
    bootstrap: vi.fn().mockResolvedValue(undefined),
    logout: vi.fn(),
    ...overrides,
  };
}

describe('LoginPage', () => {
  it('renders sign in form', () => {
    render(<LoginPage auth={makeAuth()} />);
    expect(screen.getByText('WebMux')).toBeDefined();
    expect(screen.getByLabelText('Username')).toBeDefined();
    expect(screen.getByLabelText('Password')).toBeDefined();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeDefined();
  });

  it('renders bootstrap form when bootstrap_required', () => {
    const auth = makeAuth({
      authStatus: { mode: 'local', bootstrap_required: true },
    });
    render(<LoginPage auth={auth} />);
    expect(screen.getByText('Create your first account to get started')).toBeDefined();
    expect(screen.getByLabelText('Confirm Password')).toBeDefined();
  });

  it('calls login with correct credentials', async () => {
    const auth = makeAuth();
    render(<LoginPage auth={auth} />);

    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'admin' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'secret' } });
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      expect(auth.login).toHaveBeenCalledWith('admin', 'secret');
    });
  });

  it('shows error message when auth fails', async () => {
    const auth = makeAuth({ error: 'Invalid credentials' });
    render(<LoginPage auth={auth} />);
    expect(screen.getByText('Invalid credentials')).toBeDefined();
  });

  it('shows validation error for empty password', async () => {
    const auth = makeAuth();
    render(<LoginPage auth={auth} />);
    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'admin' } });
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }));
    await waitFor(() => {
      expect(screen.getByText('Username and password are required')).toBeDefined();
    });
    expect(auth.login).not.toHaveBeenCalled();
  });
});
