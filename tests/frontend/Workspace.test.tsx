import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { Workspace } from '@frontend/components/Workspace';
import { InputBroadcastProvider } from '@frontend/contexts/InputBroadcastContext';
import type { ReactNode } from 'react';

const mockSessions = [
  {
    id: 's1', owner: 'u1', transport: 'ssh' as const, host_id: '', hostname: 'h1', username: 'u1',
    key_id: '', cols: 80, rows: 24, row: 0, col: 0, port: 22,
    state: 'connected' as const, created_at: '', updated_at: '', title: 'u1@h1', persistent: true,
  },
];

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

vi.mock('@frontend/components/Terminal', () => ({
  Terminal: vi.fn().mockImplementation(({ sessionId }: { sessionId: string }) => (
    <div data-testid={`terminal-${sessionId}`}>Terminal Mock</div>
  )),
}));

const wrapper = ({ children }: { children: ReactNode }) => (
  <InputBroadcastProvider>{children}</InputBroadcastProvider>
);

describe('Workspace', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
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
});
