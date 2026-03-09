import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';

// Mock useAuth before importing App
const mockAuth = {
  isAuthenticated: false,
  isLoading: true,
  authStatus: null as { mode: string; bootstrap_required: boolean } | null,
  error: null as string | null,
  login: vi.fn(),
  bootstrap: vi.fn(),
  logout: vi.fn(),
};

vi.mock('@frontend/hooks/useAuth', () => ({
  useAuth: () => mockAuth,
}));

vi.mock('@frontend/utils/api', () => ({
  api: {
    getConfig: vi.fn().mockResolvedValue({
      app: {
        name: 'webmux',
        http_port: 8080,
        https_port: 8443,
        secure_mode: false,
        trusted_http_allowed: true,
        default_term: { cols: 80, rows: 24, font_size: 14 },
      },
    }),
    getSessions: vi.fn().mockResolvedValue([]),
    getHosts: vi.fn().mockResolvedValue([]),
    getKeys: vi.fn().mockResolvedValue([]),
    getAuthStatus: vi.fn().mockResolvedValue({ mode: 'none', bootstrap_required: false }),
  },
}));

// Mock Terminal
vi.mock('@frontend/components/Terminal', () => ({
  Terminal: () => <div>Terminal Mock</div>,
}));

import App from '@frontend/App';

describe('App', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuth.isAuthenticated = false;
    mockAuth.isLoading = true;
    mockAuth.authStatus = null;
    mockAuth.error = null;
  });

  it('shows loading state', () => {
    render(<App />);
    expect(screen.getByText('Loading...')).toBeDefined();
  });

  it('shows login page when not authenticated', () => {
    mockAuth.isLoading = false;
    mockAuth.authStatus = { mode: 'local', bootstrap_required: false };
    render(<App />);
    expect(screen.getByText('WebMux')).toBeDefined();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeDefined();
  });

  it('shows workspace when authenticated', async () => {
    mockAuth.isLoading = false;
    mockAuth.isAuthenticated = true;
    mockAuth.authStatus = { mode: 'none', bootstrap_required: false };
    render(<App />);
    await waitFor(() => {
      expect(screen.getByText('Click to add a session')).toBeDefined();
    });
  });
});
