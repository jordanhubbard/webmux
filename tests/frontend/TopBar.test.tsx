import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { TopBar } from '@frontend/components/TopBar';
import { InputBroadcastProvider } from '@frontend/contexts/InputBroadcastContext';
import type { AuthState } from '@frontend/hooks/useAuth';
import type { ReactNode } from 'react';

const wrapper = ({ children }: { children: ReactNode }) => (
  <InputBroadcastProvider>{children}</InputBroadcastProvider>
);

function makeAuth(overrides: Partial<AuthState> = {}): AuthState {
  return {
    isAuthenticated: true,
    isLoading: false,
    authStatus: { mode: 'local', bootstrap_required: false },
    error: null,
    login: vi.fn(),
    bootstrap: vi.fn(),
    logout: vi.fn(),
    ...overrides,
  };
}

describe('TopBar', () => {
  const defaultTopBarProps = () => ({
    auth: makeAuth(),
    fontSize: 14,
    onFontSizeChange: vi.fn(),
    termCols: 80,
    termRows: 24,
    onTermSizeChange: vi.fn(),
    onNewAccount: vi.fn(),
    secureMode: true,
    currentUser: 'admin',
    globalAutoScroll: true,
    onGlobalAutoScrollChange: vi.fn(),
    globalLock: false,
    onGlobalLockChange: vi.fn(),
  });

  it('renders logo and controls', () => {
    render(<TopBar {...defaultTopBarProps()} />, { wrapper });
    expect(screen.getAllByText(/WebMux/i).length).toBeGreaterThan(0);
    expect(screen.getByText('14px')).toBeDefined();
  });

  it('shows secure badge in secure mode', () => {
    render(<TopBar {...defaultTopBarProps()} />, { wrapper });
    expect(screen.getByText(/Secure/)).toBeDefined();
  });

  it('shows trusted badge in trusted mode', () => {
    render(
      <TopBar {...defaultTopBarProps()} auth={makeAuth({ authStatus: { mode: 'none', bootstrap_required: false } })} secureMode={false} />,
      { wrapper },
    );
    expect(screen.getByText(/Trusted/)).toBeDefined();
  });

  it('calls onFontSizeChange when clicking A- or A+', () => {
    const onFontSizeChange = vi.fn();
    render(<TopBar {...defaultTopBarProps()} onFontSizeChange={onFontSizeChange} />, { wrapper });
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
    expect(screen.getByText('admin')).toBeDefined();
  });

  it('shows + Account button', () => {
    render(<TopBar {...defaultTopBarProps()} />, { wrapper });
    expect(screen.getByText('+ Account')).toBeDefined();
  });

  it('calls onNewAccount when + Account clicked', () => {
    const onNewAccount = vi.fn();
    render(<TopBar {...defaultTopBarProps()} onNewAccount={onNewAccount} />, { wrapper });
    fireEvent.click(screen.getByText('+ Account'));
    expect(onNewAccount).toHaveBeenCalled();
  });

  it('shows term size and responds to C+/C-/R+/R-', () => {
    const onTermSizeChange = vi.fn();
    render(<TopBar {...defaultTopBarProps()} onTermSizeChange={onTermSizeChange} />, { wrapper });
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
    expect(screen.getByTestId('host-switcher')).toBeDefined();
    expect(screen.getByText('Lab A')).toBeDefined();
    expect(screen.getByText('Lab B')).toBeDefined();
  });
});
