import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Tile } from '@frontend/components/Tile';
import { InputBroadcastProvider } from '@frontend/contexts/InputBroadcastContext';
import type { Session } from '@frontend/types';
import type { ReactNode } from 'react';

vi.mock('@frontend/components/Terminal', () => ({
  Terminal: vi.fn().mockImplementation(({ sessionId }: { sessionId: string }) => (
    <div data-testid={`terminal-${sessionId}`}>Terminal Mock</div>
  )),
}));

const wrapper = ({ children }: { children: ReactNode }) => (
  <InputBroadcastProvider>{children}</InputBroadcastProvider>
);

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: 's1',
    owner: 'testuser',
    transport: 'ssh',
    host_id: '',
    hostname: 'example.com',
    username: 'user',
    key_id: '',
    cols: 80,
    rows: 24,
    row: 0,
    col: 0,
    state: 'connected',
    created_at: '',
    updated_at: '',
    title: 'user@example.com',
    persistent: true,
    ...overrides,
  };
}

describe('Tile', () => {
  it('renders session title and transport badge', () => {
    render(
      <Tile
        session={makeSession()}
        fontSize={14}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
        onRename={vi.fn()}
      />,
      { wrapper },
    );
    expect(screen.getByText('user@example.com')).toBeDefined();
    expect(screen.getByText('SSH')).toBeDefined();
  });

  it('renders MOSH transport badge', () => {
    render(
      <Tile
        session={makeSession({ transport: 'mosh' })}
        fontSize={14}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
        onRename={vi.fn()}
      />,
      { wrapper },
    );
    expect(screen.getByText('MOSH')).toBeDefined();
  });

  it('calls onClose when close button clicked', () => {
    const onClose = vi.fn();
    render(
      <Tile
        session={makeSession()}
        fontSize={14}
        onClose={onClose}
        onReconnect={vi.fn()}
        onRename={vi.fn()}
      />,
      { wrapper },
    );
    fireEvent.click(screen.getByTitle('Close'));
    expect(onClose).toHaveBeenCalledWith('s1');
  });

  it('shows reconnect button for disconnected sessions', () => {
    render(
      <Tile
        session={makeSession({ state: 'disconnected' })}
        fontSize={14}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
        onRename={vi.fn()}
      />,
      { wrapper },
    );
    expect(screen.getByTitle('Reconnect')).toBeDefined();
  });

  it('calls onReconnect when reconnect button clicked', () => {
    const onReconnect = vi.fn();
    render(
      <Tile
        session={makeSession({ state: 'disconnected' })}
        fontSize={14}
        onClose={vi.fn()}
        onReconnect={onReconnect}
        onRename={vi.fn()}
      />,
      { wrapper },
    );
    fireEvent.click(screen.getByTitle('Reconnect'));
    expect(onReconnect).toHaveBeenCalledWith('s1');
  });

  it('does not show reconnect for connected sessions', () => {
    render(
      <Tile
        session={makeSession({ state: 'connected' })}
        fontSize={14}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
        onRename={vi.fn()}
      />,
      { wrapper },
    );
    expect(screen.queryByTitle('Reconnect')).toBeNull();
  });

  it('shows error state reconnect button', () => {
    render(
      <Tile
        session={makeSession({ state: 'error' })}
        fontSize={14}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
        onRename={vi.fn()}
      />,
      { wrapper },
    );
    expect(screen.getByTitle('Reconnect')).toBeDefined();
  });

  it('renders terminal mock', () => {
    render(
      <Tile
        session={makeSession()}
        fontSize={14}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
        onRename={vi.fn()}
      />,
      { wrapper },
    );
    expect(screen.getByTestId('terminal-s1')).toBeDefined();
  });

  it('renders scroll-to-bottom button', () => {
    render(
      <Tile
        session={makeSession()}
        fontSize={14}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
        onRename={vi.fn()}
      />,
      { wrapper },
    );
    expect(screen.getByTitle('Scroll to bottom')).toBeDefined();
  });

  it('calls onTitleMouseDown when chrome dragged', () => {
    const onTitleMouseDown = vi.fn();
    render(
      <Tile
        session={makeSession()}
        fontSize={14}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
        onRename={vi.fn()}
        onTitleMouseDown={onTitleMouseDown}
      />,
      { wrapper },
    );
    // Fire mousedown on the chrome (not a button) — title includes rename hint
    const chrome = screen.getByTitle('user@example.com (double-click to rename)').closest('[style]')!.parentElement!;
    fireEvent.mouseDown(chrome);
    expect(onTitleMouseDown).toHaveBeenCalledWith('s1', expect.any(Object));
  });
});
