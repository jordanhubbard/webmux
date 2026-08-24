import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { RdpTile } from '@frontend/components/RdpTile';
import type { RdpSession } from '@frontend/types';

vi.mock('@frontend/components/RdpViewer', () => ({
  RdpViewer: vi.fn().mockImplementation(
    ({ sessionId }: { sessionId: string }) => (
      <div data-testid={`rdp-viewer-${sessionId}`}>RdpViewer Mock</div>
    ),
  ),
}));

function makeRdpSession(overrides: Partial<RdpSession> = {}): RdpSession {
  return {
    id: 'r1',
    kind: 'rdp',
    owner: 'testuser',
    host_id: '',
    hostname: 'desktop.example.com',
    rdp_port: 3389,
    rdp_username: 'user',
    rdp_domain: '',
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

describe('RdpTile', () => {
  it('shows a prominent reconnect action in a disconnected RDP window', () => {
    const onReconnect = vi.fn();
    render(
      <RdpTile
        session={makeRdpSession({ state: 'disconnected' })}
        onDoubleClick={vi.fn()}
        onClose={vi.fn()}
        onReconnect={onReconnect}
      />,
    );

    fireEvent.click(screen.getByText('Reconnect'));
    expect(onReconnect).toHaveBeenCalledTimes(1);
  });

  it('shows the prominent reconnect action for an RDP error', () => {
    render(
      <RdpTile
        session={makeRdpSession({ state: 'error' })}
        onDoubleClick={vi.fn()}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
      />,
    );

    expect(screen.getByText('Reconnect')).toBeDefined();
  });

  it('hides the prominent reconnect action when RDP reconnection starts', () => {
    const props = {
      onDoubleClick: vi.fn(),
      onClose: vi.fn(),
      onReconnect: vi.fn(),
    };
    const { rerender } = render(
      <RdpTile session={makeRdpSession({ state: 'disconnected' })} {...props} />,
    );

    rerender(<RdpTile session={makeRdpSession({ state: 'connecting' })} {...props} />);
    expect(screen.queryByText('Reconnect')).toBeNull();
  });

  it('does not show the prominent reconnect action while connected', () => {
    render(
      <RdpTile
        session={makeRdpSession()}
        onDoubleClick={vi.fn()}
        onClose={vi.fn()}
        onReconnect={vi.fn()}
      />,
    );

    expect(screen.queryByText('Reconnect')).toBeNull();
  });
});
