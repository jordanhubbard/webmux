import { createRef } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { RecoveryDialog } from '@frontend/components/RecoveryDialog';
import { TerminalActions } from '@frontend/components/TerminalActions';
import type { TerminalHandle } from '@frontend/components/Terminal';
import { api } from '@frontend/utils/api';
import type { SessionRecovery } from '@frontend/types';

function evidence(sessionId = 'one'): SessionRecovery {
  return {
    session_id: sessionId, hostname: 'worker.example', transport: 'ssh', lifecycle_owner: 'webmux',
    message: 'Review the recorded activity and available history before continuing.',
    history: '<img src=x onerror="execute()">', history_available: true, shell_ready: false,
    can_resume_cli: false, can_replay: false,
    state: { revision: 4, pending: true, auto_resume: false,
      activity: { command: 'printf previous', cwd: '/project', source: 'shell-integration',
        observed_at: '2026-11-09T12:00:00Z', running: true, replayable: true } },
  };
}

afterEach(() => vi.restoreAllMocks());

describe('recorded work preview', () => {
  it('loads only after an explicit click, including while disconnected, and renders history as text', async () => {
    const load = vi.spyOn(api, 'getSessionRecovery').mockResolvedValue(evidence());
    const terminalRef = createRef<TerminalHandle>();
    render(<TerminalActions sessionId="one" terminalRef={terminalRef} connected={false} transcriptEnabled={false} />);
    expect(load).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Recover work' }));
    expect(await screen.findByText('printf previous')).toBeDefined();
    expect(screen.getByText('/project')).toBeDefined();
    expect(screen.getByText('worker.example · SSH')).toBeDefined();
    expect(screen.getByLabelText('Previous terminal output').textContent).toBe(evidence().history);
    expect(document.querySelector('img')).toBeNull();
    expect(load).toHaveBeenCalledOnce();
  });

  it('ignores an old response after switching sessions', async () => {
    let finishOld!: (value: SessionRecovery) => void;
    const load = vi.spyOn(api, 'getSessionRecovery')
      .mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve; }))
      .mockResolvedValueOnce({ ...evidence('two'), hostname: 'new-worker.example' });
    const view = render(<RecoveryDialog sessionId="one" onClose={vi.fn()} />);
    const signal = load.mock.calls[0][1];
    view.rerender(<RecoveryDialog sessionId="two" onClose={vi.fn()} />);
    expect(await screen.findByText('new-worker.example · SSH')).toBeDefined();
    await act(async () => finishOld({ ...evidence(), hostname: 'old-worker.example' }));
    expect(signal?.aborted).toBe(true);
    expect(screen.queryByText('old-worker.example · SSH')).toBeNull();
  });

  it('offers refresh after a failed read and preserves an explicit missing-command fallback', async () => {
    const value = { ...evidence(), state: undefined };
    const load = vi.spyOn(api, 'getSessionRecovery').mockRejectedValueOnce(new Error('Cannot reach server')).mockResolvedValueOnce(value);
    render(<RecoveryDialog sessionId="one" onClose={vi.fn()} />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Cannot reach server');
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(load).toHaveBeenCalledTimes(2));
    expect(await screen.findByText(/No reliable command was recorded/)).toBeDefined();
    expect(screen.getByLabelText('Previous terminal output').textContent).toBe(value.history);
  });
});
