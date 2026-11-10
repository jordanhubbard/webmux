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
  it('shows the saved managed destination and project when no shell command is known', async () => {
    const value = evidence();
    value.lifecycle_owner = 'agent-deck';
    value.manager_target = 'agent-deck:remote:original-session';
    value.manager_directory = '/remote/project';
    value.manager_profile = 'work';
    value.manager_native_session = 'recorded-native-id';
    value.manager_native_state_file = '/native/conversation.jsonl';
    value.manager_native_source = 'agent-deck-inventory';
    value.state!.activity = undefined;
    vi.spyOn(api, 'getSessionRecovery').mockResolvedValue(value);
    render(<RecoveryDialog sessionId="one" onClose={vi.fn()} />);
    expect(await screen.findByText('agent-deck:remote:original-session')).toBeDefined();
    expect(screen.getByText('/remote/project')).toBeDefined();
    expect(screen.getByText('work')).toBeDefined();
    expect(screen.getByText('recorded-native-id')).toBeDefined();
    expect(screen.getByText('/native/conversation.jsonl')).toBeDefined();
    expect(screen.getByText(/Later conversation changes have not been verified/)).toBeDefined();
    expect(screen.queryByText('Last observed command')).toBeNull();
  });

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

function waitingEvidence(): SessionRecovery {
  const value = evidence();
  return { ...value, waiting: true, host_revision: 17, instance: 'waiting-worker', can_resume_cli: true, can_replay: true,
    state: { ...value.state!, activity: { ...value.state!.activity!, cli: 'codex', native_session_id: 'native-id' } } };
}

describe('native conversation evidence', () => {
  it.each([
    ['launch-selector', /Selected at launch.*automatic resume is unavailable/],
    ['process-state', /Observed in the running CLI process/],
    ['native-hook', /Reported by the coding CLI.*automatic resume is unavailable/],
    [undefined, /current conversation could not be verified/],
  ] as const)('explains %s evidence without treating a recorded selector as current state', async (source, explanation) => {
    const value = waitingEvidence();
    value.state!.activity!.native_evidence = source;
    vi.spyOn(api, 'getSessionRecovery').mockResolvedValue(value);
    render(<RecoveryDialog sessionId="one" onClose={vi.fn()} />);
    expect(await screen.findByText(explanation)).toBeDefined();
    expect(screen.getByText('native-id')).toBeDefined();
    expect(screen.getByRole('button', { name: 'Resume codex conversation' })).toBeEnabled();
  });
});

describe('continuation approval', () => {
  it('clears managed approval when the selected terminal changes', async () => {
    const value: SessionRecovery = { ...evidence(), waiting: false, host_revision: 0, instance: 'managed-identity',
      lifecycle_owner: 'agent-deck', can_resume_managed: true, manager_native_session: 'recorded-conversation' };
    vi.spyOn(api, 'getSessionRecovery').mockResolvedValue(value);
    const recover = vi.spyOn(api, 'recoverSession');
    const dialog = render(<RecoveryDialog sessionId="one" onClose={vi.fn()} />);
    const approve = await screen.findByLabelText(/I approve asking Agent Deck to resume/);
    fireEvent.click(approve);
    expect(screen.getByRole('button', { name: 'Resume through Agent Deck' })).toBeEnabled();
    dialog.rerender(<RecoveryDialog sessionId="two" onClose={vi.fn()} />);
    expect(await screen.findByRole('button', { name: 'Resume through Agent Deck' })).toBeDisabled();
    expect(screen.getByLabelText(/I approve asking Agent Deck to resume/)).not.toBeChecked();
    expect(recover).not.toHaveBeenCalled();
  });

  it('requires explicit managed approval and sends no transcript or arbitrary command', async () => {
    const value: SessionRecovery = { ...evidence(), waiting: false, host_revision: 0, instance: 'managed-identity',
      lifecycle_owner: 'agent-deck', can_resume_managed: true, manager_native_session: 'recorded-conversation',
      manager_directory: '/project', manager_target: 'agent-deck:local:seat' };
    value.state!.activity = undefined;
    vi.spyOn(api, 'getSessionRecovery').mockResolvedValue(value);
    const recover = vi.spyOn(api, 'recoverSession').mockResolvedValue({ outcome: 'consumed' });
    render(<RecoveryDialog sessionId="one" onClose={vi.fn()} />);
    const resume = await screen.findByRole('button', { name: 'Resume through Agent Deck' });
    expect(resume).toBeDisabled();
    expect(screen.queryByRole('button', { name: 'Start a shell' })).toBeNull();
    fireEvent.click(screen.getByLabelText(/I approve asking Agent Deck to resume/));
    fireEvent.click(resume);
    await waitFor(() => expect(recover).toHaveBeenCalledOnce());
    expect(recover).toHaveBeenCalledWith('one', expect.objectContaining({ kind: 'managed', confirm: true,
      revision: 4, host_revision: 0, instance: 'managed-identity' }));
    expect(recover.mock.calls[0][1]).not.toHaveProperty('context');
    expect(recover.mock.calls[0][1]).not.toHaveProperty('command');
  });

  it('resumes the selected native conversation without transmitting history by default', async () => {
    vi.spyOn(api, 'getSessionRecovery').mockResolvedValue(waitingEvidence());
    const recover = vi.spyOn(api, 'recoverSession').mockResolvedValue({ outcome: 'consumed' });
    const close = vi.fn();
    render(<RecoveryDialog sessionId="one" onClose={close} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Resume codex conversation' }));
    await waitFor(() => expect(close).toHaveBeenCalledOnce());
    expect(recover).toHaveBeenCalledWith('one', expect.objectContaining({ kind: 'native', revision: 4, host_revision: 17, instance: 'waiting-worker', confirm: false }));
    expect(recover.mock.calls[0][1]).not.toHaveProperty('context');
  });

  it('requires approval before replay and keeps transcript consent separate', async () => {
    vi.spyOn(api, 'getSessionRecovery').mockResolvedValue(waitingEvidence());
    const recover = vi.spyOn(api, 'recoverSession').mockResolvedValue({ outcome: 'consumed' });
    render(<RecoveryDialog sessionId="one" onClose={vi.fn()} />);
    const run = await screen.findByRole('button', { name: 'Run recorded command' });
    expect(run).toBeDisabled();
    expect(recover).not.toHaveBeenCalled();
    fireEvent.click(screen.getByLabelText(/I approve running the displayed command/));
    expect(run).toBeEnabled();
    expect(screen.getByLabelText(/I approve sharing the selected terminal output/)).not.toBeChecked();
    fireEvent.click(run);
    await waitFor(() => expect(recover).toHaveBeenCalledWith('one', expect.objectContaining({ kind: 'replay', confirm: true })));
    expect(recover.mock.calls[0][1]).not.toHaveProperty('context');
  });

  it('transmits only selected text after handoff approval and a resume click', async () => {
    vi.spyOn(api, 'getSessionRecovery').mockResolvedValue(waitingEvidence());
    const recover = vi.spyOn(api, 'recoverSession').mockResolvedValue({ outcome: 'consumed' });
    render(<RecoveryDialog sessionId="one" onClose={vi.fn()} />);
    fireEvent.click(await screen.findByLabelText(/I approve sharing the selected terminal output/));
    fireEvent.change(screen.getByLabelText('Selected terminal output to share'), { target: { value: 'selected context only' } });
    expect(recover).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Resume codex conversation' }));
    await waitFor(() => expect(recover).toHaveBeenCalledWith('one', expect.objectContaining({ kind: 'native', confirm: true, context: 'selected context only' })));
  });

  it('saves automatic resume only after opting in', async () => {
    vi.spyOn(api, 'getSessionRecovery').mockResolvedValue(waitingEvidence());
    const save = vi.spyOn(api, 'setAutoRecovery').mockResolvedValue({ auto_resume: true });
    render(<RecoveryDialog sessionId="one" onClose={vi.fn()} />);
    const automatic = await screen.findByLabelText(/Automatically resume interrupted native coding conversations/);
    expect(automatic).not.toBeChecked();
    expect(save).not.toHaveBeenCalled();
    fireEvent.click(automatic);
    await waitFor(() => expect(save).toHaveBeenCalledWith('one', true));
  });
});


it('shows a Rig checkpoint and manual destination without offering an unqualified launch', async () => {
  const value = evidence();
  value.lifecycle_owner = 'rig'; value.state!.activity = undefined;
  value.manager_target = 'coder@project'; value.manager_profile = 'default';
  value.manager_native_session = 'native-conversation'; value.manager_snapshot_id = 'rig-checkpoint';
  vi.spyOn(api, 'getSessionRecovery').mockResolvedValue(value);
  const recover = vi.spyOn(api, 'recoverSession');
  render(<RecoveryDialog sessionId="one" onClose={vi.fn()} />);
  expect(await screen.findByText('rig-checkpoint')).toBeVisible();
  expect(screen.getByText('native-conversation')).toBeVisible();
  expect(screen.getByText(/Resume this recorded conversation through its session manager/)).toBeVisible();
  expect(screen.queryByRole('button', { name: /Resume through/ })).toBeNull();
  expect(recover).not.toHaveBeenCalled();
});
