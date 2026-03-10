import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

vi.mock('@frontend/utils/api', () => ({
  api: {
    register: vi.fn(),
  },
}));

import { RegisterDialog } from '@frontend/components/RegisterDialog';

// Helper: the form has 3 inputs — username (text), password, confirm password (both type=password)
function getInputs() {
  const username = screen.getByPlaceholderText('work');
  const passwordInputs = document.querySelectorAll<HTMLInputElement>('input[type="password"]');
  return {
    username,
    password: passwordInputs[0],
    confirmPassword: passwordInputs[1],
  };
}

function fillForm(username: string, password: string, confirm: string) {
  const inputs = getInputs();
  fireEvent.change(inputs.username, { target: { value: username } });
  fireEvent.change(inputs.password, { target: { value: password } });
  fireEvent.change(inputs.confirmPassword, { target: { value: confirm } });
}

describe('RegisterDialog', () => {
  const defaultProps = () => ({
    onClose: vi.fn(),
    onCreated: vi.fn(),
  });

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders form fields and buttons', () => {
    render(<RegisterDialog {...defaultProps()} />);
    expect(screen.getByText('Create New Account')).toBeDefined();
    expect(screen.getByPlaceholderText('work')).toBeDefined();
    expect(screen.getByText('Username')).toBeDefined();
    expect(screen.getByText('Password')).toBeDefined();
    expect(screen.getByText('Confirm Password')).toBeDefined();
    expect(screen.getByRole('button', { name: /create account/i })).toBeDefined();
    expect(screen.getByRole('button', { name: /cancel/i })).toBeDefined();
  });

  it('shows error when username is empty', () => {
    render(<RegisterDialog {...defaultProps()} />);
    fireEvent.click(screen.getByRole('button', { name: /create account/i }));
    expect(screen.getByText('Username is required')).toBeDefined();
  });

  it('shows error when username is too short', () => {
    render(<RegisterDialog {...defaultProps()} />);
    fireEvent.change(screen.getByPlaceholderText('work'), { target: { value: 'a' } });
    fireEvent.click(screen.getByRole('button', { name: /create account/i }));
    expect(screen.getByText('Username must be at least 2 characters')).toBeDefined();
  });

  it('shows error when password is empty', () => {
    render(<RegisterDialog {...defaultProps()} />);
    fireEvent.change(screen.getByPlaceholderText('work'), { target: { value: 'testuser' } });
    fireEvent.click(screen.getByRole('button', { name: /create account/i }));
    expect(screen.getByText('Password is required')).toBeDefined();
  });

  it('shows error when password is too short', () => {
    render(<RegisterDialog {...defaultProps()} />);
    fillForm('testuser', 'abc', 'abc');
    fireEvent.click(screen.getByRole('button', { name: /create account/i }));
    expect(screen.getByText('Password must be at least 4 characters')).toBeDefined();
  });

  it('shows error when passwords do not match', () => {
    render(<RegisterDialog {...defaultProps()} />);
    fillForm('testuser', 'pass1234', 'different');
    fireEvent.click(screen.getByRole('button', { name: /create account/i }));
    expect(screen.getByText('Passwords do not match')).toBeDefined();
  });

  it('calls api.register and onCreated on success', async () => {
    const { api } = await import('@frontend/utils/api');
    (api.register as ReturnType<typeof vi.fn>).mockResolvedValue({ username: 'testuser' });
    const props = defaultProps();

    render(<RegisterDialog {...props} />);
    fillForm('testuser', 'pass1234', 'pass1234');
    fireEvent.click(screen.getByRole('button', { name: /create account/i }));

    await waitFor(() => {
      expect(api.register).toHaveBeenCalledWith('testuser', 'pass1234');
      expect(props.onCreated).toHaveBeenCalledWith('testuser');
    });
  });

  it('shows API error on failure', async () => {
    const { api } = await import('@frontend/utils/api');
    (api.register as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('Username taken'));

    render(<RegisterDialog {...defaultProps()} />);
    fillForm('testuser', 'pass1234', 'pass1234');
    fireEvent.click(screen.getByRole('button', { name: /create account/i }));

    await waitFor(() => {
      expect(screen.getByText('Username taken')).toBeDefined();
    });
  });

  it('calls onClose when cancel is clicked', () => {
    const props = defaultProps();
    render(<RegisterDialog {...props} />);
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }));
    expect(props.onClose).toHaveBeenCalled();
  });

  it('trims username before submitting', async () => {
    const { api } = await import('@frontend/utils/api');
    (api.register as ReturnType<typeof vi.fn>).mockResolvedValue({ username: 'trimmed' });
    const props = defaultProps();

    render(<RegisterDialog {...props} />);
    fillForm('  trimmed  ', 'pass1234', 'pass1234');
    fireEvent.click(screen.getByRole('button', { name: /create account/i }));

    await waitFor(() => {
      expect(api.register).toHaveBeenCalledWith('trimmed', 'pass1234');
    });
  });
});
