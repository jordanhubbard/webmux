import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { Workspace } from '../components/Workspace';

const mockSessions = [
  {
    id: 's1', transport: 'ssh' as const, host_id: '', hostname: 'h1', username: 'u1',
    key_id: '', cols: 80, rows: 24, row: 0, col: 0,
    state: 'connected' as const, created_at: '', updated_at: '', title: 'u1@h1', persistent: true,
  },
];

vi.mock('../utils/api', () => ({
  api: {
    getSessions: vi.fn().mockResolvedValue([]),
    createSession: vi.fn(),
    deleteSession: vi.fn(),
    reconnectSession: vi.fn(),
    splitRight: vi.fn(),
    splitBelow: vi.fn(),
    getHosts: vi.fn().mockResolvedValue([]),
    getKeys: vi.fn().mockResolvedValue([]),
  },
}));

// Mock Terminal which needs xterm.js
vi.mock('../components/Terminal', () => ({
  Terminal: ({ sessionId }: { sessionId: string }) => (
    <div data-testid={`terminal-${sessionId}`}>Terminal Mock</div>
  ),
}));

describe('Workspace', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows empty state when no sessions', async () => {
    render(<Workspace fontSize={14} showAddDialog={false} onDialogClose={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByText('No active sessions')).toBeDefined();
    });
  });

  it('shows loading state initially', () => {
    render(<Workspace fontSize={14} showAddDialog={false} onDialogClose={vi.fn()} />);
    expect(screen.getByText('Loading sessions…')).toBeDefined();
  });

  it('renders sessions as tiles', async () => {
    const { api } = await import('../utils/api');
    (api.getSessions as ReturnType<typeof vi.fn>).mockResolvedValue(mockSessions);

    render(<Workspace fontSize={14} showAddDialog={false} onDialogClose={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByText('u1@h1')).toBeDefined();
    });
  });

  it('shows connection dialog when showAddDialog is true', async () => {
    render(<Workspace fontSize={14} showAddDialog={true} onDialogClose={vi.fn()} />);
    await waitFor(() => {
      expect(screen.getByText('New SSH Session')).toBeDefined();
    });
  });
});
