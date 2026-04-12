import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { VncTile } from '@frontend/components/VncTile';
import type { VncSession } from '@frontend/types';

// VncViewer opens a real WebSocket; replace it with a simple div.
vi.mock('@frontend/components/VncViewer', () => ({
  VncViewer: vi.fn().mockImplementation(
    ({ sessionId }: { sessionId: string }) => (
      <div data-testid={`vnc-viewer-${sessionId}`}>VncViewer Mock</div>
    ),
  ),
}));

function makeVncSession(overrides: Partial<VncSession> = {}): VncSession {
  return {
    id: 'v1',
    kind: 'vnc',
    owner: 'testuser',
    host_id: '',
    hostname: 'desktop.example.com',
    vnc_port: 5900,
    row: 0,
    col: 0,
    state: 'connected',
    created_at: '',
    updated_at: '',
    title: 'desktop.example.com',
    persistent: true,
    ...overrides,
  };
}

describe('VncTile', () => {
  it('renders the session hostname in the title area', () => {
    render(
      <VncTile
        session={makeVncSession()}
        onDoubleClick={vi.fn()}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
      />,
    );
    expect(screen.getByTitle('desktop.example.com')).toBeDefined();
  });

  it('renders a VNC transport badge', () => {
    render(
      <VncTile
        session={makeVncSession()}
        onDoubleClick={vi.fn()}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
      />,
    );
    expect(screen.getByText('VNC')).toBeDefined();
  });

  it('calls onClose when the close button is clicked', () => {
    const onClose = vi.fn();
    render(
      <VncTile
        session={makeVncSession()}
        onDoubleClick={vi.fn()}
        onClose={onClose}
        onReconnect={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByTitle('Close'));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('shows the reconnect button when state is disconnected', () => {
    render(
      <VncTile
        session={makeVncSession({ state: 'disconnected' })}
        onDoubleClick={vi.fn()}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
      />,
    );
    expect(screen.getByTitle('Reconnect')).toBeDefined();
  });

  it('calls onReconnect when the reconnect button is clicked', () => {
    const onReconnect = vi.fn();
    render(
      <VncTile
        session={makeVncSession({ state: 'disconnected' })}
        onDoubleClick={vi.fn()}
        onClose={vi.fn()}
        onReconnect={onReconnect}
      />,
    );
    fireEvent.click(screen.getByTitle('Reconnect'));
    expect(onReconnect).toHaveBeenCalledTimes(1);
  });

  it('does not show the reconnect button when state is connected', () => {
    render(
      <VncTile
        session={makeVncSession({ state: 'connected' })}
        onDoubleClick={vi.fn()}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
      />,
    );
    expect(screen.queryByTitle('Reconnect')).toBeNull();
  });

  it('shows the reconnect button when state is error', () => {
    render(
      <VncTile
        session={makeVncSession({ state: 'error' })}
        onDoubleClick={vi.fn()}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
      />,
    );
    expect(screen.getByTitle('Reconnect')).toBeDefined();
  });

  it('calls onDoubleClick when the body is double-clicked', () => {
    const onDoubleClick = vi.fn();
    render(
      <VncTile
        session={makeVncSession()}
        onDoubleClick={onDoubleClick}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
      />,
    );
    fireEvent.doubleClick(screen.getByTestId('vnc-viewer-v1'));
    expect(onDoubleClick).toHaveBeenCalledTimes(1);
  });

  it('renders the VncViewer mock', () => {
    render(
      <VncTile
        session={makeVncSession()}
        onDoubleClick={vi.fn()}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
      />,
    );
    expect(screen.getByTestId('vnc-viewer-v1')).toBeDefined();
  });
});
