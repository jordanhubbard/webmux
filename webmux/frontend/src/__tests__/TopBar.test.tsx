import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { TopBar } from '../components/TopBar';
import { InputBroadcastProvider } from '../contexts/InputBroadcastContext';
import type { AuthState } from '../hooks/useAuth';
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
  it('renders logo and controls', () => {
    render(
      <TopBar
        auth={makeAuth()}
        fontSize={14}
        onFontSizeChange={vi.fn()}
        onAddSession={vi.fn()}
        secureMode={true}
      />,
      { wrapper },
    );
    expect(screen.getAllByText(/WebMux/i).length).toBeGreaterThan(0);
    expect(screen.getByText('+ New Session')).toBeDefined();
    expect(screen.getByText('14px')).toBeDefined();
  });

  it('calls onAddSession when clicking + New Session', () => {
    const onAddSession = vi.fn();
    render(
      <TopBar
        auth={makeAuth()}
        fontSize={14}
        onFontSizeChange={vi.fn()}
        onAddSession={onAddSession}
        secureMode={true}
      />,
      { wrapper },
    );
    fireEvent.click(screen.getByText('+ New Session'));
    expect(onAddSession).toHaveBeenCalled();
  });

  it('shows secure badge in secure mode', () => {
    render(
      <TopBar
        auth={makeAuth()}
        fontSize={14}
        onFontSizeChange={vi.fn()}
        onAddSession={vi.fn()}
        secureMode={true}
      />,
      { wrapper },
    );
    expect(screen.getByText(/Secure/)).toBeDefined();
  });

  it('shows trusted badge in trusted mode', () => {
    render(
      <TopBar
        auth={makeAuth({ authStatus: { mode: 'none', bootstrap_required: false } })}
        fontSize={14}
        onFontSizeChange={vi.fn()}
        onAddSession={vi.fn()}
        secureMode={false}
      />,
      { wrapper },
    );
    expect(screen.getByText(/Trusted/)).toBeDefined();
  });

  it('calls onFontSizeChange when clicking A- or A+', () => {
    const onFontSizeChange = vi.fn();
    render(
      <TopBar
        auth={makeAuth()}
        fontSize={14}
        onFontSizeChange={onFontSizeChange}
        onAddSession={vi.fn()}
        secureMode={true}
      />,
      { wrapper },
    );
    fireEvent.click(screen.getByText('A+'));
    expect(onFontSizeChange).toHaveBeenCalledWith(15);
    fireEvent.click(screen.getByText('A-'));
    expect(onFontSizeChange).toHaveBeenCalledWith(13);
  });

  it('shows Type to All button', () => {
    render(
      <TopBar
        auth={makeAuth()}
        fontSize={14}
        onFontSizeChange={vi.fn()}
        onAddSession={vi.fn()}
        secureMode={true}
      />,
      { wrapper },
    );
    expect(screen.getByText('Type to All')).toBeDefined();
  });

  it('toggles Type to All button on click', () => {
    render(
      <TopBar
        auth={makeAuth()}
        fontSize={14}
        onFontSizeChange={vi.fn()}
        onAddSession={vi.fn()}
        secureMode={true}
      />,
      { wrapper },
    );
    fireEvent.click(screen.getByText('Type to All'));
    expect(screen.getByText('Type to All: ON')).toBeDefined();
  });
});
