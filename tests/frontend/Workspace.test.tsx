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

  const defaultProps = { fontSize: 14, termCols: 80, termRows: 24 };

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
      expect(screen.getByText('u1@h1')).toBeDefined();
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

  it('cycles terminal focus left-to-right then down and wraps', async () => {
    const { api } = await import('@frontend/utils/api');
    const sessions = [
      { ...mockSessions[0], id: 's1', title: 'one', row: 0, col: 1 },
      { ...mockSessions[0], id: 's2', title: 'two', row: 0, col: 0 },
      { ...mockSessions[0], id: 's3', title: 'three', row: 1, col: 0 },
    ];
    (api.getSessions as ReturnType<typeof vi.fn>).mockResolvedValue(sessions);

    const originalOffsetParent = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetParent');
    Object.defineProperty(HTMLElement.prototype, 'offsetParent', {
      configurable: true,
      get() { return document.body; },
    });

    try {
      render(<Workspace {...defaultProps} />, { wrapper });
      await waitFor(() => {
        expect(screen.getByText('three')).toBeDefined();
      });

      fireEvent.keyDown(window, { code: 'Period', ctrlKey: true, shiftKey: true });
      await waitFor(() => {
        expect(terminalFocusFns.get('s2')).toHaveBeenCalledTimes(1);
      });

      fireEvent.keyDown(window, { code: 'Period', ctrlKey: true, shiftKey: true });
      await waitFor(() => {
        expect(terminalFocusFns.get('s1')).toHaveBeenCalledTimes(1);
      });

      fireEvent.keyDown(window, { code: 'Period', ctrlKey: true, shiftKey: true });
      await waitFor(() => {
        expect(terminalFocusFns.get('s3')).toHaveBeenCalledTimes(1);
      });

      fireEvent.keyDown(window, { code: 'Period', ctrlKey: true, shiftKey: true });
      await waitFor(() => {
        expect(terminalFocusFns.get('s2')).toHaveBeenCalledTimes(2);
      });

      fireEvent.keyDown(window, { code: 'Comma', ctrlKey: true, shiftKey: true });
      await waitFor(() => {
        expect(terminalFocusFns.get('s3')).toHaveBeenCalledTimes(2);
      });

      fireEvent.keyDown(window, { key: '>', ctrlKey: true, shiftKey: true });
      await waitFor(() => {
        expect(terminalFocusFns.get('s2')).toHaveBeenCalledTimes(3);
      });

      fireEvent.keyDown(window, { key: '<', ctrlKey: true, shiftKey: true });
      await waitFor(() => {
        expect(terminalFocusFns.get('s3')).toHaveBeenCalledTimes(3);
      });
    } finally {
      if (originalOffsetParent) {
        Object.defineProperty(HTMLElement.prototype, 'offsetParent', originalOffsetParent);
      } else {
        delete (HTMLElement.prototype as unknown as Record<string, unknown>).offsetParent;
      }
    }
  });
});
