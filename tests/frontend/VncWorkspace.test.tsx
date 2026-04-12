import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { VncWorkspace } from '@frontend/components/VncWorkspace';
import type { VncSession, CreateVncSessionRequest } from '@frontend/types';

const mockVncSessions: VncSession[] = [
  {
    id: 'v1',
    kind: 'vnc',
    owner: 'testuser',
    host_id: '',
    hostname: 'desktop1.example.com',
    vnc_port: 5900,
    row: 0,
    col: 0,
    state: 'connected',
    created_at: '',
    updated_at: '',
    title: 'desktop1.example.com',
    persistent: true,
  },
  {
    id: 'v2',
    kind: 'vnc',
    owner: 'testuser',
    host_id: '',
    hostname: 'desktop2.example.com',
    vnc_port: 5901,
    row: 0,
    col: 1,
    state: 'disconnected',
    created_at: '',
    updated_at: '',
    title: 'desktop2.example.com',
    persistent: true,
  },
];

// Mock the api module — must be declared before any imports that use it.
vi.mock('@frontend/utils/api', () => ({
  api: {
    getVncSessions: vi.fn().mockResolvedValue([]),
    createVncSession: vi.fn(),
    deleteVncSession: vi.fn().mockResolvedValue(undefined),
    reconnectVncSession: vi.fn(),
    moveVncSession: vi.fn().mockResolvedValue({}),
  },
}));

// VncViewer needs a real WebSocket — swap it for a simple div.
vi.mock('@frontend/components/VncViewer', () => ({
  VncViewer: vi.fn().mockImplementation(
    ({ sessionId }: { sessionId: string }) => (
      <div data-testid={`vnc-viewer-${sessionId}`}>VncViewer Mock</div>
    ),
  ),
}));

// VncConnectionDialog: render a stub that immediately calls onConnect so we
// can test the "dialog submits" path, and exposes a Cancel button for the
// "close without connecting" path.
vi.mock('@frontend/components/VncConnectionDialog', () => ({
  VncConnectionDialog: vi.fn().mockImplementation(
    ({
      onConnect,
      onClose,
      suggestedRow,
      suggestedCol,
    }: {
      onConnect: (req: CreateVncSessionRequest, password: string) => Promise<void>;
      onClose: () => void;
      suggestedRow?: number;
      suggestedCol?: number;
    }) => (
      <div data-testid="vnc-connection-dialog">
        <span>Connect to VNC Desktop</span>
        <button
          onClick={() =>
            onConnect(
              { hostname: 'new.example.com', vnc_port: 5900, row: suggestedRow ?? 0, col: suggestedCol ?? 0 },
              '',
            )
          }
        >
          Connect
        </button>
        <button onClick={onClose}>Cancel</button>
      </div>
    ),
  ),
}));

// VncFullscreen is not the focus of these tests; stub it out.
vi.mock('@frontend/components/VncFullscreen', () => ({
  VncFullscreen: vi.fn().mockImplementation(() => (
    <div data-testid="vnc-fullscreen">VncFullscreen Mock</div>
  )),
}));

describe('VncWorkspace', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    const { api } = await import('@frontend/utils/api');
    (api.getVncSessions as ReturnType<typeof vi.fn>).mockResolvedValue([]);
  });

  it('shows loading state initially', async () => {
    const { api } = await import('@frontend/utils/api');
    (api.getVncSessions as ReturnType<typeof vi.fn>).mockReturnValue(
      new Promise(() => {}),
    );

    render(<VncWorkspace />);
    expect(screen.getByText(/Loading sessions/)).toBeDefined();
  });

  it('shows add cell when no sessions', async () => {
    render(<VncWorkspace />);
    await waitFor(() => {
      expect(screen.getByText('Click to add a session')).toBeDefined();
    });
  });

  it('renders a VncTile for each fetched session', async () => {
    const { api } = await import('@frontend/utils/api');
    (api.getVncSessions as ReturnType<typeof vi.fn>).mockResolvedValue(mockVncSessions);

    render(<VncWorkspace />);
    await waitFor(() => {
      expect(screen.getByTitle('desktop1.example.com')).toBeDefined();
      expect(screen.getByTitle('desktop2.example.com')).toBeDefined();
    });
  });

  it('opens VncConnectionDialog when an AddCell is clicked', async () => {
    render(<VncWorkspace />);
    await waitFor(() => {
      expect(screen.getByTestId('add-cell-0-0')).toBeDefined();
    });

    fireEvent.click(screen.getByTestId('add-cell-0-0'));
    await waitFor(() => {
      expect(screen.getByText('Connect to VNC Desktop')).toBeDefined();
    });
  });

  it('closing the dialog without connecting leaves sessions unchanged', async () => {
    render(<VncWorkspace />);
    await waitFor(() => {
      expect(screen.getByTestId('add-cell-0-0')).toBeDefined();
    });

    fireEvent.click(screen.getByTestId('add-cell-0-0'));
    await waitFor(() => {
      expect(screen.getByTestId('vnc-connection-dialog')).toBeDefined();
    });

    fireEvent.click(screen.getByText('Cancel'));
    await waitFor(() => {
      // Dialog gone
      expect(screen.queryByTestId('vnc-connection-dialog')).toBeNull();
      // Still no tiles — sessions unchanged
      expect(screen.queryByTitle('new.example.com')).toBeNull();
    });
  });

  it('connecting via the dialog adds a tile for the new session', async () => {
    const newSession: VncSession = {
      id: 'v-new',
      kind: 'vnc',
      owner: 'testuser',
      host_id: '',
      hostname: 'new.example.com',
      vnc_port: 5900,
      row: 0,
      col: 0,
      state: 'connected',
      created_at: '',
      updated_at: '',
      title: 'new.example.com',
      persistent: true,
    };
    const { api } = await import('@frontend/utils/api');
    (api.createVncSession as ReturnType<typeof vi.fn>).mockResolvedValue(newSession);

    render(<VncWorkspace />);
    await waitFor(() => {
      expect(screen.getByTestId('add-cell-0-0')).toBeDefined();
    });

    fireEvent.click(screen.getByTestId('add-cell-0-0'));
    await waitFor(() => {
      expect(screen.getByText('Connect')).toBeDefined();
    });

    fireEvent.click(screen.getByText('Connect'));
    await waitFor(() => {
      expect(screen.getByTitle('new.example.com')).toBeDefined();
    });
  });

  it('deleting a session calls api.deleteVncSession and removes the tile', async () => {
    const { api } = await import('@frontend/utils/api');
    (api.getVncSessions as ReturnType<typeof vi.fn>).mockResolvedValue([mockVncSessions[0]]);
    (api.deleteVncSession as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);

    render(<VncWorkspace />);
    await waitFor(() => {
      expect(screen.getByTitle('desktop1.example.com')).toBeDefined();
    });

    fireEvent.click(screen.getByTitle('Close'));
    await waitFor(() => {
      expect(api.deleteVncSession).toHaveBeenCalledWith('v1');
      expect(screen.queryByTitle('desktop1.example.com')).toBeNull();
    });
  });

  it('reconnect calls api.reconnectVncSession and updates the tile state', async () => {
    const updatedSession: VncSession = { ...mockVncSessions[1], state: 'connected' };
    const { api } = await import('@frontend/utils/api');
    (api.getVncSessions as ReturnType<typeof vi.fn>).mockResolvedValue([mockVncSessions[1]]);
    (api.reconnectVncSession as ReturnType<typeof vi.fn>).mockResolvedValue(updatedSession);

    render(<VncWorkspace />);
    await waitFor(() => {
      expect(screen.getByTitle('Reconnect')).toBeDefined();
    });

    fireEvent.click(screen.getByTitle('Reconnect'));
    await waitFor(() => {
      expect(api.reconnectVncSession).toHaveBeenCalledWith('v2');
    });
  });
});
