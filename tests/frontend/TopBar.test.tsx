import { beforeEach, describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { TopBar } from '@frontend/components/TopBar';
import { WorkspacePaneProvider } from '@frontend/contexts/WorkspacePaneContext';
import { InputBroadcastProvider } from '@frontend/contexts/InputBroadcastContext';
import type { AuthState } from '@frontend/hooks/useAuth';
import type { ReactNode } from 'react';

const apiMocks = vi.hoisted(() => ({ getCoordination: vi.fn(), updateConfig: vi.fn() }));
vi.mock('@frontend/utils/api', () => ({ api: { getCoordination: apiMocks.getCoordination, updateConfig: apiMocks.updateConfig, getMacSetup: () => Promise.resolve({ status: { supported: false } }) } }));

beforeEach(() => {
  apiMocks.getCoordination.mockReset();
  apiMocks.getCoordination.mockResolvedValue({ attentionCount: 0 });
  apiMocks.updateConfig.mockReset();
  apiMocks.updateConfig.mockResolvedValue(undefined);
});

const wrapper = ({ children }: { children: ReactNode }) => (
  <WorkspacePaneProvider availablePanes={['terminals', 'desktops', 'coordination']}><InputBroadcastProvider>{children}</InputBroadcastProvider></WorkspacePaneProvider>
);

function makeAuth(overrides: Partial<AuthState> = {}): AuthState {
  return {
    isAuthenticated: true,
    isLoading: false,
    authStatus: { mode: 'local', bootstrap_required: false },
    error: null,
    username: 'admin',
    isAdmin: true,
    sessionExpiresAt: null,
    sessionExpired: false,
    isRefreshing: false,
    login: vi.fn(),
    bootstrap: vi.fn(),
    refreshSession: vi.fn(),
    logout: vi.fn(),
    ...overrides,
  };
}

describe('TopBar', () => {
  const defaultTopBarProps = () => ({
    auth: makeAuth(),
    appName: 'WebMux',
    fontSize: 14,
    onFontSizeChange: vi.fn(),
    termCols: 80,
    termRows: 24,
    onTermSizeChange: vi.fn(),
    onManageUsers: vi.fn(),
    onManageSettings: vi.fn(),
    canManageSettings: true,
    secureMode: true,
    currentUser: 'admin',
    globalAutoScroll: true,
    onGlobalAutoScrollChange: vi.fn(),
    globalLock: false,
    onGlobalLockChange: vi.fn(),
  });

  it('opens USB setup and restores focus when closed', () => {
    render(<TopBar {...defaultTopBarProps()} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'More' }));
    const button = screen.getByRole('button', { name: 'USB redirection' });
    button.focus();
    fireEvent.click(button);
    expect(screen.getByRole('dialog', { name: 'USB redirection' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Close usb redirection' }));
    expect(screen.queryByRole('dialog', { name: 'USB redirection' })).not.toBeInTheDocument();
    expect(button).toHaveFocus();
  });

  it('renders logo and controls', () => {
    render(<TopBar {...defaultTopBarProps()} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'Display' }));
    expect(screen.getAllByText(/WebMux/i).length).toBeGreaterThan(0);
    expect(screen.getByText('14px')).toBeDefined();
  });

  it('keeps coordinator inventories and secondary controls off the terminal home', () => {
    render(<TopBar {...defaultTopBarProps()} />, { wrapper });
    expect(screen.queryByRole('combobox', { name: 'Coordinator' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'USB redirection' })).toBeNull();
    expect(screen.queryByText('14px')).toBeNull();
    expect(screen.queryByText('Sign out')).toBeNull();
    expect(apiMocks.getCoordination).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Coordination' }));
    expect(screen.getByRole('button', { name: 'Coordination' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('shows secure badge in secure mode', () => {
    render(<TopBar {...defaultTopBarProps()} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'More' }));
    expect(screen.getByText(/Secure/)).toBeDefined();
  });

  it('shows trusted badge in trusted mode', () => {
    render(
      <TopBar {...defaultTopBarProps()} auth={makeAuth({ authStatus: { mode: 'none', bootstrap_required: false } })} secureMode={false} />,
      { wrapper },
    );
    fireEvent.click(screen.getByRole('button', { name: 'More' }));
    expect(screen.getByText(/Trusted/)).toBeDefined();
  });

  it('calls onFontSizeChange when clicking A- or A+', () => {
    const onFontSizeChange = vi.fn();
    render(<TopBar {...defaultTopBarProps()} onFontSizeChange={onFontSizeChange} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'Display' }));
    fireEvent.click(screen.getByText('A+'));
    expect(onFontSizeChange).toHaveBeenCalledWith(15);
    fireEvent.click(screen.getByText('A-'));
    expect(onFontSizeChange).toHaveBeenCalledWith(13);
  });

  it('shows Type to All button', () => {
    render(<TopBar {...defaultTopBarProps()} />, { wrapper });
    expect(screen.getByText('Type to All')).toBeDefined();
  });

  it('toggles Type to All button on click', () => {
    render(<TopBar {...defaultTopBarProps()} />, { wrapper });
    fireEvent.click(screen.getByText('Type to All'));
    expect(screen.getByText('Type to All: ON')).toBeDefined();
  });

  it('shows current user badge', () => {
    render(<TopBar {...defaultTopBarProps()} currentUser="admin" />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'More' }));
    expect(screen.getByText('admin')).toBeDefined();
  });

  it('shows the session countdown and refreshes it on request', () => {
    const refreshSession = vi.fn().mockResolvedValue(undefined);
    render(
      <TopBar {...defaultTopBarProps()} auth={makeAuth({ sessionExpiresAt: Date.now() + 60_000, refreshSession })} />,
      { wrapper },
    );
    fireEvent.click(screen.getByRole('button', { name: 'More' }));
    expect(screen.getByText(/Session 00:01:00/)).toBeDefined();
    fireEvent.click(screen.getByRole('button', { name: 'Refresh session' }));
    expect(refreshSession).toHaveBeenCalledTimes(1);
  });

  it('shows Users button for admins', () => {
    render(<TopBar {...defaultTopBarProps()} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'More' }));
    expect(screen.getByText('Users')).toBeDefined();
  });

  it('hides Users button for non-admins', () => {
    render(<TopBar {...defaultTopBarProps()} auth={makeAuth({ isAdmin: false })} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'More' }));
    expect(screen.queryByText('Users')).toBeNull();
  });

  it('calls onManageUsers when Users clicked', () => {
    const onManageUsers = vi.fn();
    render(<TopBar {...defaultTopBarProps()} onManageUsers={onManageUsers} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'More' }));
    fireEvent.click(screen.getByText('Users'));
    expect(onManageUsers).toHaveBeenCalled();
  });

  it('shows the settings gear to operators and opens settings', () => {
    const onManageSettings = vi.fn();
    render(<TopBar {...defaultTopBarProps()} onManageSettings={onManageSettings} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'More' }));

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }));

    expect(onManageSettings).toHaveBeenCalledTimes(1);
  });

  it('hides the settings gear from non-admin users', () => {
    render(<TopBar {...defaultTopBarProps()} canManageSettings={false} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'More' }));
    expect(screen.queryByRole('button', { name: 'Settings' })).toBeNull();
  });

  it('shows term size and responds to C+/C-/R+/R-', () => {
    const onTermSizeChange = vi.fn();
    render(<TopBar {...defaultTopBarProps()} onTermSizeChange={onTermSizeChange} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'Display' }));
    expect(screen.getByText('80×24')).toBeDefined();
    fireEvent.click(screen.getByText('C+'));
    expect(onTermSizeChange).toHaveBeenCalledWith(90, 24);
    fireEvent.click(screen.getByText('C-'));
    expect(onTermSizeChange).toHaveBeenCalledWith(70, 24);
    fireEvent.click(screen.getByText('R+'));
    expect(onTermSizeChange).toHaveBeenCalledWith(80, 29);
    fireEvent.click(screen.getByText('R-'));
    expect(onTermSizeChange).toHaveBeenCalledWith(80, 19);
  });

  it('does not show agent buttons without configured definitions', () => {
    render(<TopBar {...defaultTopBarProps()} />, { wrapper });
    expect(screen.queryByText('Agents')).toBeNull();
  });

  it('shows a combined Agents button for configured agents', () => {
    render(
      <TopBar
        {...defaultTopBarProps()}
        agentDefinitions={[{
          id: 'codex',
          label: 'Codex',
          plural_label: 'Codex Sessions',
          badge: 'CODEX',
          tmux_socket: 'codex',
          workspace: 'agent-codex',
          enabled: true,
        }]}
        combinedAgentPane={true}
      />,
      { wrapper },
    );
    expect(screen.getByText('Agents')).toBeDefined();
    expect(screen.queryByText('Codex Sessions')).toBeNull();
  });

  it('shows configured per-agent buttons when combined pane is disabled', () => {
    render(
      <TopBar
        {...defaultTopBarProps()}
        agentDefinitions={[{
          id: 'codex',
          label: 'Codex',
          plural_label: 'Codex Sessions',
          badge: 'CODEX',
          tmux_socket: 'codex',
          workspace: 'agent-codex',
          enabled: true,
        }]}
        combinedAgentPane={false}
      />,
      { wrapper },
    );
    expect(screen.queryByText('Agents')).toBeNull();
    expect(screen.getByText('Codex Sessions')).toBeDefined();
  });

  it('renders a config-driven host switcher only when enabled', () => {
    render(
      <TopBar
        {...defaultTopBarProps()}
        hostSwitcher={{
          enabled: true,
          suffixes: [],
          hosts: [
            { id: 'lab-a', label: 'Lab A', hostname: 'lab-a-webmux.example.net' },
            { id: 'lab-b', label: 'Lab B', hostname: 'lab-b-webmux.example.net' },
          ],
        }}
      />,
      { wrapper },
    );
    fireEvent.click(screen.getByRole('button', { name: 'More' }));
    expect(screen.getByTestId('host-switcher')).toBeDefined();
    expect(screen.getByText('Lab A')).toBeDefined();
    expect(screen.getByText('Lab B')).toBeDefined();
  });
});
