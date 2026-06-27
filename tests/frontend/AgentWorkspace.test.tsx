import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, render, screen, waitFor, fireEvent } from '@testing-library/react';
import type { AgentDefinition, Session } from '@frontend/types';
import { AgentWorkspace } from '@frontend/components/AgentWorkspace';

const apiMock = vi.hoisted(() => ({
  getAllAgentSessions: vi.fn(),
  getAgentSessions: vi.fn(),
  attachAgentSession: vi.fn(),
  createAgentScratch: vi.fn(),
  deleteSession: vi.fn(),
}));

vi.mock('@frontend/utils/api', () => ({
  api: apiMock,
}));

vi.mock('@frontend/components/Terminal', () => ({
  Terminal: ({ sessionId }: { sessionId: string }) => <div data-testid={`terminal-${sessionId}`}>Terminal {sessionId}</div>,
}));

const codexDefinition: AgentDefinition = {
  id: 'codex',
  label: 'Codex',
  plural_label: 'Codex Sessions',
  badge: 'CODEX',
  tmux_socket: 'codex',
  workspace: 'agent-codex',
  enabled: true,
};

const helperDefinition: AgentDefinition = {
  id: 'helper',
  label: 'Helper',
  plural_label: 'Helper Sessions',
  badge: 'HELP',
  tmux_socket: 'helper',
  workspace: 'agent-helper',
  enabled: true,
};

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: 'agent-session-1',
    kind: 'terminal',
    owner: 'anonymous',
    transport: 'exec',
    host_id: '',
    hostname: 'codex.local',
    port: 0,
    username: 'codex',
    key_id: '',
    cols: 120,
    rows: 40,
    row: 0,
    col: 0,
    state: 'connected',
    created_at: '',
    updated_at: '',
    title: 'codex-a',
    persistent: false,
    minimized: false,
    workspace: 'agent-codex',
    agent_id: 'codex',
    agent_role: 'attach',
    agent_session_name: 'codex-a',
    ...overrides,
  };
}

function makeAgentSession(agentId: string, name: string, overrides: Record<string, unknown> = {}) {
  return {
    name,
    agent_id: agentId,
    display_name: name,
    windows: 1,
    attached: 0,
    created_at: '2026-06-14T22:08:55.000Z',
    last_output_at: '2026-06-17T20:00:00.000Z',
    status: 'waiting',
    status_source: 'hook',
    ...overrides,
  };
}

const defaultProps = {
  fontSize: 14,
  termCols: 120,
  termRows: 40,
  themes: [],
  globalTheme: null,
};

const originalMatchMedia = window.matchMedia;
const originalVisualViewport = Object.getOwnPropertyDescriptor(window, 'visualViewport');
const originalMaxTouchPoints = Object.getOwnPropertyDescriptor(Navigator.prototype, 'maxTouchPoints');

function setTouchViewport(width: number, height: number) {
  Object.defineProperty(window, 'visualViewport', {
    configurable: true,
    value: {
      width,
      height,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    },
  });
  Object.defineProperty(Navigator.prototype, 'maxTouchPoints', {
    configurable: true,
    value: 5,
  });
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: query === '(pointer: coarse)',
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));
}

function restoreViewport() {
  window.matchMedia = originalMatchMedia;
  if (originalVisualViewport) {
    Object.defineProperty(window, 'visualViewport', originalVisualViewport);
  } else {
    delete (window as Partial<Window>).visualViewport;
  }
  if (originalMaxTouchPoints) {
    Object.defineProperty(Navigator.prototype, 'maxTouchPoints', originalMaxTouchPoints);
  } else {
    delete (Navigator.prototype as Partial<Navigator>).maxTouchPoints;
  }
}

describe('AgentWorkspace', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiMock.getAllAgentSessions.mockResolvedValue([makeAgentSession('codex', 'codex-a')]);
    apiMock.getAgentSessions.mockResolvedValue([makeAgentSession('codex', 'codex-a')]);
    apiMock.attachAgentSession.mockResolvedValue(makeSession());
    apiMock.createAgentScratch.mockResolvedValue(makeSession({
      id: 'agent-scratch-1',
      title: 'Scratch shell',
      agent_role: 'scratch',
      agent_session_name: undefined,
    }));
    apiMock.deleteSession.mockResolvedValue(undefined);
  });

  afterEach(() => {
    restoreViewport();
  });

  it('auto-selects the first configured agent session and requests attach', async () => {
    render(
      <AgentWorkspace
        agent={codexDefinition}
        agentDefinitions={[codexDefinition]}
        {...defaultProps}
      />,
    );

    await waitFor(() => {
      expect(apiMock.attachAgentSession).toHaveBeenCalledWith('codex', { name: 'codex-a', cols: 120, rows: 40 });
    });
    expect(await screen.findByTestId('terminal-agent-session-1')).toBeDefined();
    expect(screen.getByTestId('agent-layout-codex')).toHaveStyle('grid-template-columns: minmax(0, 1fr)');
    expect(apiMock.createAgentScratch).not.toHaveBeenCalled();
  });

  it('uses a viewport-sized attach request on touch devices', async () => {
    setTouchViewport(780, 650);

    render(
      <AgentWorkspace
        agent={codexDefinition}
        agentDefinitions={[codexDefinition]}
        {...defaultProps}
        termCols={190}
        termRows={44}
      />,
    );

    await waitFor(() => {
      expect(apiMock.attachAgentSession).toHaveBeenCalled();
    });

    const [, request] = apiMock.attachAgentSession.mock.calls[0];
    expect(request.name).toBe('codex-a');
    expect(request.cols).toBeLessThan(190);
    expect(request.cols).toBeGreaterThanOrEqual(40);
    expect(request.rows).toBeLessThanOrEqual(44);
    expect(request.rows).toBeGreaterThanOrEqual(10);
  });

  it('opens and closes a scratch shell beside the selected agent session', async () => {
    render(
      <AgentWorkspace
        agent={codexDefinition}
        agentDefinitions={[codexDefinition]}
        {...defaultProps}
      />,
    );

    await screen.findByTestId('terminal-agent-session-1');
    fireEvent.click(screen.getByText('+ Shell'));

    await waitFor(() => {
      expect(apiMock.createAgentScratch).toHaveBeenCalledWith('codex', { selectedName: 'codex-a', cols: 60, rows: 40 });
    });
    expect(screen.getByTestId('agent-layout-codex')).toHaveStyle('grid-template-columns: minmax(0, 2fr) minmax(0, 1fr)');
    expect(await screen.findByTestId('terminal-agent-scratch-1')).toBeDefined();

    fireEvent.click(screen.getByTitle('Close scratch shell'));

    await waitFor(() => {
      expect(apiMock.deleteSession).toHaveBeenCalledWith('agent-scratch-1');
    });
    expect(screen.getByTestId('agent-layout-codex')).toHaveStyle('grid-template-columns: minmax(0, 1fr)');
  });

  it('renders combined configured agents and attaches by selected agent id', async () => {
    apiMock.getAllAgentSessions.mockResolvedValue([
      makeAgentSession('codex', 'codex-a', { display_name: 'codex-a' }),
      makeAgentSession('helper', 'helper-a', { display_name: 'helper-a' }),
    ]);
    apiMock.attachAgentSession
      .mockResolvedValueOnce(makeSession({ id: 'codex-session', title: 'codex-a', agent_id: 'codex', agent_session_name: 'codex-a' }))
      .mockResolvedValueOnce(makeSession({
        id: 'helper-session',
        title: 'helper-a',
        username: 'helper',
        hostname: 'helper.local',
        workspace: 'agent-helper',
        agent_id: 'helper',
        agent_session_name: 'helper-a',
      }));

    render(
      <AgentWorkspace
        agentDefinitions={[codexDefinition, helperDefinition]}
        {...defaultProps}
      />,
    );

    await waitFor(() => {
      expect(apiMock.attachAgentSession).toHaveBeenCalledWith('codex', { name: 'codex-a', cols: 120, rows: 40 });
    });
    expect(screen.getAllByText('codex-a').length).toBeGreaterThan(0);
    expect(screen.getByText('helper-a')).toBeDefined();

    fireEvent.click(screen.getByText('helper-a'));

    await waitFor(() => {
      expect(apiMock.attachAgentSession).toHaveBeenCalledWith('helper', { name: 'helper-a', cols: 120, rows: 40 });
    });
    expect(await screen.findByTestId('terminal-helper-session')).toBeDefined();
  });

  it('shows an empty state without opening scratch in a combined pane until a session is selected', async () => {
    apiMock.getAllAgentSessions.mockResolvedValue([]);

    render(
      <AgentWorkspace
        agentDefinitions={[codexDefinition, helperDefinition]}
        {...defaultProps}
      />,
    );

    expect(await screen.findByText('No agent sessions')).toBeDefined();
    expect(screen.getByText('+ Shell')).toBeDisabled();
    expect(apiMock.attachAgentSession).not.toHaveBeenCalled();
    expect(apiMock.createAgentScratch).not.toHaveBeenCalled();
  });

  it('refreshes per-agent panes on an interval', async () => {
    vi.useFakeTimers();
    try {
      render(
        <AgentWorkspace
          agent={codexDefinition}
          agentDefinitions={[codexDefinition]}
          {...defaultProps}
        />,
      );

      await act(async () => {
        await Promise.resolve();
      });
      expect(apiMock.getAgentSessions).toHaveBeenCalledTimes(1);

      await act(async () => {
        vi.advanceTimersByTime(10000);
        await Promise.resolve();
      });

      expect(apiMock.getAgentSessions).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });
});
