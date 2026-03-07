import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Tile } from '../components/Tile';
import { InputBroadcastProvider } from '../contexts/InputBroadcastContext';
import type { Session } from '../types';
import type { ReactNode } from 'react';

// Mock Terminal which needs xterm.js
vi.mock('../components/Terminal', () => ({
  Terminal: ({ sessionId }: { sessionId: string }) => (
    <div data-testid={`terminal-${sessionId}`}>Terminal Mock</div>
  ),
}));

const wrapper = ({ children }: { children: ReactNode }) => (
  <InputBroadcastProvider>{children}</InputBroadcastProvider>
);

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: 's1',
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
        onSplitRight={vi.fn()}
        onSplitBelow={vi.fn()}
        onReconnect={vi.fn()}
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
        onSplitRight={vi.fn()}
        onSplitBelow={vi.fn()}
        onReconnect={vi.fn()}
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
        onSplitRight={vi.fn()}
        onSplitBelow={vi.fn()}
        onReconnect={vi.fn()}
      />,
      { wrapper },
    );
    fireEvent.click(screen.getByTitle('Close'));
    expect(onClose).toHaveBeenCalledWith('s1');
  });

  it('calls onSplitRight when split right button clicked', () => {
    const onSplitRight = vi.fn();
    render(
      <Tile
        session={makeSession()}
        fontSize={14}
        onClose={vi.fn()}
        onSplitRight={onSplitRight}
        onSplitBelow={vi.fn()}
        onReconnect={vi.fn()}
      />,
      { wrapper },
    );
    fireEvent.click(screen.getByTitle('Split right'));
    expect(onSplitRight).toHaveBeenCalledWith('s1');
  });

  it('calls onSplitBelow when split below button clicked', () => {
    const onSplitBelow = vi.fn();
    render(
      <Tile
        session={makeSession()}
        fontSize={14}
        onClose={vi.fn()}
        onSplitRight={vi.fn()}
        onSplitBelow={onSplitBelow}
        onReconnect={vi.fn()}
      />,
      { wrapper },
    );
    fireEvent.click(screen.getByTitle('Split below'));
    expect(onSplitBelow).toHaveBeenCalledWith('s1');
  });

  it('shows reconnect button for disconnected sessions', () => {
    render(
      <Tile
        session={makeSession({ state: 'disconnected' })}
        fontSize={14}
        onClose={vi.fn()}
        onSplitRight={vi.fn()}
        onSplitBelow={vi.fn()}
        onReconnect={vi.fn()}
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
        onSplitRight={vi.fn()}
        onSplitBelow={vi.fn()}
        onReconnect={onReconnect}
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
        onSplitRight={vi.fn()}
        onSplitBelow={vi.fn()}
        onReconnect={vi.fn()}
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
        onSplitRight={vi.fn()}
        onSplitBelow={vi.fn()}
        onReconnect={vi.fn()}
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
        onSplitRight={vi.fn()}
        onSplitBelow={vi.fn()}
        onReconnect={vi.fn()}
      />,
      { wrapper },
    );
    expect(screen.getByTestId('terminal-s1')).toBeDefined();
  });
});
