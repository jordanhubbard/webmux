import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { Workspace } from '@frontend/components/Workspace';
import { InputBroadcastProvider } from '@frontend/contexts/InputBroadcastContext';
import type { ReactNode, Ref } from 'react';

const mockSessions = [
  {
    id: 's1', owner: 'u1', transport: 'ssh' as const, host_id: '', hostname: 'h1', username: 'u1',
    key_id: '', cols: 80, rows: 24, row: 0, col: 0, port: 22,
    state: 'connected' as const, created_at: '', updated_at: '', title: 'u1@h1', persistent: true,
  },
];

const terminalFocusFns = vi.hoisted(() => new Map<string, ReturnType<typeof vi.fn>>());

vi.mock('@frontend/utils/api', () => ({
  api: {
    getSessions: vi.fn().mockResolvedValue([]),
    createSession: vi.fn(),
    deleteSession: vi.fn(),
    reconnectSession: vi.fn(),
    moveSession: vi.fn().mockResolvedValue({}),
    getHosts: vi.fn().mockResolvedValue([]),
    getKeys: vi.fn().mockResolvedValue([]),
  },
}));

vi.mock('@frontend/components/Terminal', async () => {
  const React = await vi.importActual<typeof import('react')>('react');
  const { useInputBroadcast } = await vi.importActual<typeof import('@frontend/contexts/InputBroadcastContext')>(
    '@frontend/contexts/InputBroadcastContext',
  );
  return {
    Terminal: React.forwardRef(function TerminalMock(
      { sessionId }: { sessionId: string },
      ref: Ref<unknown>,
    ) {
      const { setFocusedSessionId } = useInputBroadcast();
      const focus = terminalFocusFns.get(sessionId) ?? vi.fn();
      terminalFocusFns.set(sessionId, focus);
      React.useImperativeHandle(ref, () => ({
        scrollToBottom: vi.fn(),
        isAtBottom: () => true,
        sendInput: vi.fn(),
        focus,
      }));
      return (
        <div data-testid={`terminal-${sessionId}`} onMouseDown={() => setFocusedSessionId(sessionId)}>
          Terminal Mock
        </div>
      );
    }),
  };
});

const wrapper = ({ children }: { children: ReactNode }) => (
  <InputBroadcastProvider>{children}</InputBroadcastProvider>
);

function rect(left: number, top: number, right: number, bottom: number): DOMRect {
  return {
    left,
    top,
    right,
    bottom,
    x: left,
    y: top,
    width: right - left,
    height: bottom - top,
    toJSON: () => ({}),
  } as DOMRect;
}

describe('Workspace', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    terminalFocusFns.clear();
    const { api } = await import('@frontend/utils/api');
    (api.getSessions as ReturnType<typeof vi.fn>).mockResolvedValue([]);
  });

  const defaultProps = { fontSize: 14, termCols: 80, termRows: 24, globalAutoScroll: true, globalAutoScrollVersion: 0, onGlobalAutoScrollChange: vi.fn(), globalLock: false, globalLockVersion: 0, onGlobalLockChange: vi.fn() };

  it('shows add cell when no sessions', async () => {
    render(<Workspace {...defaultProps} />, { wrapper });
    await waitFor(() => {
      expect(screen.getByText('Click to add a session')).toBeDefined();
    });
  });

  it('shows loading state initially', async () => {
    const { api } = await import('@frontend/utils/api');
    (api.getSessions as ReturnType<typeof vi.fn>).mockReturnValue(new Promise(() => {}));

    render(<Workspace {...defaultProps} />, { wrapper });
    expect(screen.getByText(/Loading sessions/)).toBeDefined();
  });

  it('renders sessions as tiles', async () => {
    const { api } = await import('@frontend/utils/api');
    (api.getSessions as ReturnType<typeof vi.fn>).mockResolvedValue(mockSessions);

    render(<Workspace {...defaultProps} />, { wrapper });
    await waitFor(() => {
      expect(screen.getAllByText('u1@h1').length).toBeGreaterThan(0);
    });
  });

  it('shows add cells adjacent to existing tiles', async () => {
    const { api } = await import('@frontend/utils/api');
    (api.getSessions as ReturnType<typeof vi.fn>).mockResolvedValue(mockSessions);

    render(<Workspace {...defaultProps} />, { wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('add-cell-0-1')).toBeDefined();
      expect(screen.getByTestId('add-cell-1-0')).toBeDefined();
    });
  });

  it('hides add cells outside the terminal grid limit', async () => {
    const { api } = await import('@frontend/utils/api');
    (api.getSessions as ReturnType<typeof vi.fn>).mockResolvedValue(mockSessions);

    render(<Workspace {...defaultProps} terminalGridLimit={{ maxCols: 1, maxRows: 1 }} />, { wrapper });
    await waitFor(() => {
      expect(screen.getAllByText('u1@h1').length).toBeGreaterThan(0);
    });
    expect(screen.queryByTestId('add-cell-0-1')).toBeNull();
    expect(screen.queryByTestId('add-cell-1-0')).toBeNull();
  });

  it('opens connection dialog when add cell clicked', async () => {
    const { api } = await import('@frontend/utils/api');
    (api.getSessions as ReturnType<typeof vi.fn>).mockResolvedValue([]);

    render(<Workspace {...defaultProps} />, { wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('add-cell-0-0')).toBeDefined();
    });
    fireEvent.click(screen.getByTestId('add-cell-0-0'));
    await waitFor(() => {
      expect(screen.getByText('Connect to Host')).toBeDefined();
    });
  });

  it('scrolls the focused terminal tile fully into view', async () => {
    const { api } = await import('@frontend/utils/api');
    const sessions = [
      { ...mockSessions[0], id: 's1', title: 'one', row: 0, col: 0 },
      { ...mockSessions[0], id: 's2', title: 'two', row: 0, col: 1 },
    ];
    (api.getSessions as ReturnType<typeof vi.fn>).mockResolvedValue(sessions);

    render(<Workspace {...defaultProps} />, { wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('tile-cell-s2')).toBeDefined();
    });

    const workspace = screen.getByTestId('workspace-scroll');
    const tile = screen.getByTestId('tile-cell-s2');
    workspace.scrollLeft = 10;
    workspace.scrollTop = 5;

    const workspaceRect = vi.spyOn(workspace, 'getBoundingClientRect')
      .mockReturnValue(rect(0, 0, 100, 100));
    const tileRect = vi.spyOn(tile, 'getBoundingClientRect')
      .mockImplementation(() => rect(
        130 - workspace.scrollLeft,
        135 - workspace.scrollTop,
        230 - workspace.scrollLeft,
        235 - workspace.scrollTop,
      ));

    fireEvent.mouseDown(screen.getByTestId('terminal-s2'));

    await waitFor(() => {
      expect(workspace.scrollLeft).toBe(130);
      expect(workspace.scrollTop).toBe(135);
    });

    workspaceRect.mockRestore();
    tileRect.mockRestore();
  });

  it('highlights the dock icon for the focused terminal', async () => {
    const { api } = await import('@frontend/utils/api');
    const sessions = [
      { ...mockSessions[0], id: 's1', title: 'one', row: 0, col: 0 },
      { ...mockSessions[0], id: 's2', title: 'two', row: 0, col: 1 },
    ];
    (api.getSessions as ReturnType<typeof vi.fn>).mockResolvedValue(sessions);

    render(<Workspace {...defaultProps} />, { wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('dock-s1')).toBeDefined();
      expect(screen.getByTestId('dock-s2')).toBeDefined();
    });

    expect(screen.getByTestId('dock-s1').getAttribute('data-focused')).toBe('false');
    expect(screen.getByTestId('dock-s2').getAttribute('data-focused')).toBe('false');

    fireEvent.mouseDown(screen.getByTestId('terminal-s2'));

    await waitFor(() => {
      expect(screen.getByTestId('dock-s2').getAttribute('data-focused')).toBe('true');
    });
    expect(screen.getByTestId('dock-s1').getAttribute('data-focused')).toBe('false');

    fireEvent.mouseDown(screen.getByTestId('terminal-s1'));

    await waitFor(() => {
      expect(screen.getByTestId('dock-s1').getAttribute('data-focused')).toBe('true');
    });
    expect(screen.getByTestId('dock-s2').getAttribute('data-focused')).toBe('false');
  });

  it('does not scroll when the focused terminal tile is already fully visible', async () => {
    const { api } = await import('@frontend/utils/api');
    const sessions = [
      { ...mockSessions[0], id: 's1', title: 'one', row: 0, col: 0 },
      { ...mockSessions[0], id: 's2', title: 'two', row: 0, col: 1 },
    ];
    (api.getSessions as ReturnType<typeof vi.fn>).mockResolvedValue(sessions);

    render(<Workspace {...defaultProps} />, { wrapper });
    await waitFor(() => {
      expect(screen.getByTestId('tile-cell-s2')).toBeDefined();
    });

    const workspace = screen.getByTestId('workspace-scroll');
    const tile = screen.getByTestId('tile-cell-s2');
    workspace.scrollLeft = 40;
    workspace.scrollTop = 55;

    const workspaceRect = vi.spyOn(workspace, 'getBoundingClientRect')
      .mockReturnValue(rect(0, 0, 100, 100));
    const tileRect = vi.spyOn(tile, 'getBoundingClientRect')
      .mockReturnValue(rect(10, 15, 90, 95));

    fireEvent.mouseDown(screen.getByTestId('terminal-s2'));

    await waitFor(() => {
      expect(workspace.scrollLeft).toBe(40);
      expect(workspace.scrollTop).toBe(55);
    });

    workspaceRect.mockRestore();
    tileRect.mockRestore();
  });

  it('leaves application control keys untouched and keeps click-to-focus available', async () => {
    const { api } = await import('@frontend/utils/api');
    (api.getSessions as ReturnType<typeof vi.fn>).mockResolvedValue(mockSessions);
    render(<Workspace {...defaultProps} />, { wrapper });
    const terminal = await screen.findByTestId('terminal-s1');
    await waitFor(() => expect(terminalFocusFns.has('s1')).toBe(true));

    for (const code of ['KeyF', 'KeyL', 'Comma', 'Period']) {
      const event = new KeyboardEvent('keydown', { code, ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true });
      fireEvent(terminal, event);
      expect(event.defaultPrevented).toBe(false);
    }
    expect(terminalFocusFns.get('s1')).not.toHaveBeenCalled();
    fireEvent.mouseDown(terminal);
    expect(screen.getByTestId('dock-s1')).toHaveAttribute('data-focused', 'true');
  });
});
