import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { api } from '@frontend/utils/api';
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
    kind: 'terminal',
    minimized: false,
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
  it('keeps host and browser helpers behind session tools without hiding terminal actions', () => {
    render(<Tile session={makeSession()} fontSize={14} onClose={vi.fn()} onReconnect={vi.fn()} onRename={vi.fn()} />, { wrapper });
    expect(screen.queryByRole('button', { name: 'Open browser' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Sign in' })).toBeNull();
    const tools = screen.getByRole('button', { name: 'Session tools' });
    tools.focus();
    fireEvent.click(tools);
    expect(screen.getByRole('dialog', { name: 'Session tools' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Open browser' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Close session tools' }));
    expect(tools).toHaveFocus();
  });

  it('renders session title and transport badge', () => {
    render(
      <Tile
        session={makeSession()}
        fontSize={14}
        autoScroll={true}
        onAutoScrollToggle={vi.fn()}
        locked={false}
        onLockToggle={vi.fn()}
        collapsed={false}
        onToggleCollapse={vi.fn()}
        onBell={vi.fn()}
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
        autoScroll={true}
        onAutoScrollToggle={vi.fn()}
        locked={false}
        onLockToggle={vi.fn()}
        collapsed={false}
        onToggleCollapse={vi.fn()}
        onBell={vi.fn()}
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
        autoScroll={true}
        onAutoScrollToggle={vi.fn()}
        locked={false}
        onLockToggle={vi.fn()}
        collapsed={false}
        onToggleCollapse={vi.fn()}
        onBell={vi.fn()}
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

  it('shows a prominent reconnect action in a disconnected terminal', () => {
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

    const reconnect = screen.getByRole('button', { name: 'Reconnect' });
    expect(reconnect.textContent).toBe('\u21bb');
    fireEvent.click(reconnect);
    expect(onReconnect).toHaveBeenCalledWith('s1');
  });

  it('hides the prominent reconnect action when reconnection starts', () => {
    const props = {
      fontSize: 14,
      onClose: vi.fn(),
      onReconnect: vi.fn(),
      onRename: vi.fn(),
    };
    const { rerender } = render(
      <Tile session={makeSession({ state: 'disconnected' })} {...props} />,
      { wrapper },
    );

    rerender(<Tile session={makeSession({ state: 'connecting' })} {...props} />);
    expect(screen.queryByText('\u21bb')).toBeNull();
  });

  it('does not show reconnect for connected sessions', () => {
    render(
      <Tile
        session={makeSession({ state: 'connected' })}
        fontSize={14}
        autoScroll={true}
        onAutoScrollToggle={vi.fn()}
        locked={false}
        onLockToggle={vi.fn()}
        collapsed={false}
        onToggleCollapse={vi.fn()}
        onBell={vi.fn()}
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
        autoScroll={true}
        onAutoScrollToggle={vi.fn()}
        locked={false}
        onLockToggle={vi.fn()}
        collapsed={false}
        onToggleCollapse={vi.fn()}
        onBell={vi.fn()}
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
        autoScroll={true}
        onAutoScrollToggle={vi.fn()}
        locked={false}
        onLockToggle={vi.fn()}
        collapsed={false}
        onToggleCollapse={vi.fn()}
        onBell={vi.fn()}
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
        autoScroll={true}
        onAutoScrollToggle={vi.fn()}
        locked={false}
        onLockToggle={vi.fn()}
        collapsed={false}
        onToggleCollapse={vi.fn()}
        onBell={vi.fn()}
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
        autoScroll={true}
        onAutoScrollToggle={vi.fn()}
        locked={false}
        onLockToggle={vi.fn()}
        collapsed={false}
        onToggleCollapse={vi.fn()}
        onBell={vi.fn()}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
        onRename={vi.fn()}
        onTitleMouseDown={onTitleMouseDown}
      />,
      { wrapper },
    );
    // Fire mousedown on the chrome (not a button) — title includes rename hint
    const chrome = screen.getByTitle(/Project: not set/).closest('[style]')!.parentElement!;
    fireEvent.mouseDown(chrome);
    expect(onTitleMouseDown).toHaveBeenCalledWith('s1', expect.any(Object));
  });
});

it('shows explicit Rig provenance separately from title and transport', () => {
  const session = makeSession({ title: 'My shell title', rig: { target: 'opaque-id', project: 'demo-release', name: 'reviewer', host: 'build-host', runtime: 'codex' } });
  const view = render(<Tile session={session} fontSize={14} onClose={vi.fn()} onReconnect={vi.fn()} onRename={vi.fn()} />, { wrapper });
  expect(screen.getByText('demo-release / My shell title')).toBeVisible();
  expect(screen.getByText('SSH')).toBeVisible();
  expect(screen.getByLabelText('Rig-managed worker')).toHaveTextContent('Rig');
  expect(screen.getByLabelText('Rig-managed worker')).toHaveAttribute('title', expect.stringContaining('demo-release'));
  view.rerender(<Tile session={makeSession({ title: 'Rig demo-release reviewer build-host' })} fontSize={14} onClose={vi.fn()} onReconnect={vi.fn()} onRename={vi.fn()} />);
  expect(screen.queryByLabelText('Rig-managed worker')).toBeNull();
});

it('saves an explicit project label for an unmanaged session and shows it as identity', async () => {
  const saved = vi.spyOn(api, 'setSessionProject').mockResolvedValue(makeSession({ project_label: 'WebMux' }));
  const updated = vi.fn();
  try {
    render(<Tile session={makeSession()} fontSize={14} onClose={vi.fn()} onReconnect={vi.fn()} onRename={vi.fn()} onSessionUpdated={updated} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'Set project' }));
    fireEvent.change(screen.getByLabelText('Project name'), { target: { value: 'WebMux' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save project label' }));
    await waitFor(() => expect(saved).toHaveBeenCalledWith('s1', 'WebMux'));
    expect(await screen.findByText('WebMux / user@example.com')).toBeVisible();
    expect(updated).toHaveBeenCalledWith(expect.objectContaining({ project_label: 'WebMux' }));
    expect(screen.queryByRole('button', { name: 'Set project' })).toBeNull();
  } finally { saved.mockRestore(); }
});
