import { afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { RigStopDialog, type RigStopPreview } from '@frontend/components/RigStopDialog';
import type { CoordinationSeat } from '@frontend/components/CoordinationDialog';
import { api } from '@frontend/utils/api';

const seat = { id: 'local-rig/stable', projectName: 'Project', name: 'worker' } as CoordinationSeat;
const preview: RigStopPreview = { id: 'a'.repeat(64), target: seat.id, reason: 'Task complete', expiresAt: new Date(Date.now() + 180000).toISOString(), project: 'rig', seat: 'agents.worker', sessionName: 'worker@project', runtime: 'terminal', tmuxServer: '/tmp/private.sock', nodeId: 'node', sessionId: 'session', effect: 'Stop the local tmux seat; remote providers may continue.' };
afterEach(() => vi.restoreAllMocks());

it('reviews the current destination and requires separate explicit confirmation', async () => {
  const review = vi.spyOn(api, 'previewRigStop').mockResolvedValue(preview);
  const execute = vi.spyOn(api, 'executeRigStop').mockResolvedValue({ id: preview.id, state: 'stopped', code: 'stopped', posted: true, stopPhase: 'kill_attempted' });
  const complete = vi.fn();
  render(<RigStopDialog seat={seat} onClose={vi.fn()} onComplete={complete} />);
  fireEvent.change(screen.getByLabelText('Reason for stopping'), { target: { value: 'Task complete' } });
  fireEvent.click(screen.getByRole('button', { name: 'Review local Stop' }));
  await screen.findByText(preview.effect);
  expect(review).toHaveBeenCalledWith(seat.id, 'Task complete');
  expect(execute).not.toHaveBeenCalled();
  const button = screen.getByRole('button', { name: 'Confirm local seat Stop' });
  expect(button).toBeDisabled();
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.click(button);
  await screen.findByRole('status');
  expect(screen.getByRole('status')).toHaveTextContent('Rig confirmed that the reviewed local seat stopped');
  expect(execute).toHaveBeenCalledExactlyOnceWith(preview.id);
  expect(complete).toHaveBeenCalledTimes(1);
  expect(screen.queryByRole('button', { name: 'Confirm local seat Stop' })).toBeNull();
});

it('keeps a lost response uncertain and never offers an automatic retry', async () => {
  vi.spyOn(api, 'previewRigStop').mockResolvedValue(preview);
  const execute = vi.spyOn(api, 'executeRigStop').mockRejectedValue(new Error('connection lost'));
  render(<RigStopDialog seat={seat} onClose={vi.fn()} onComplete={vi.fn()} />);
  fireEvent.change(screen.getByLabelText('Reason for stopping'), { target: { value: preview.reason } });
  fireEvent.click(screen.getByRole('button', { name: 'Review local Stop' }));
  await screen.findByRole('checkbox');
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.click(screen.getByRole('button', { name: 'Confirm local seat Stop' }));
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('outcome is unconfirmed'));
  expect(screen.getByRole('status')).toHaveTextContent('will not be retried');
  expect(screen.queryByRole('button', { name: /Confirm|Retry/ })).toBeNull();
  expect(execute).toHaveBeenCalledTimes(1);
});

it('distinguishes a detach from a confirmed kill in the native receipt', async () => {
  vi.spyOn(api, 'previewRigStop').mockResolvedValue(preview);
  vi.spyOn(api, 'executeRigStop').mockResolvedValue({ id: preview.id, state: 'uncertain', code: 'selection_changed', posted: true, stopPhase: 'detached' });
  render(<RigStopDialog seat={seat} onClose={vi.fn()} onComplete={vi.fn()} />);
  fireEvent.change(screen.getByLabelText('Reason for stopping'), { target: { value: preview.reason } });
  fireEvent.click(screen.getByRole('button', { name: 'Review local Stop' }));
  await screen.findByRole('checkbox');
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.click(screen.getByRole('button', { name: 'Confirm local seat Stop' }));
  expect(await screen.findByRole('status')).toHaveTextContent('Rig disconnected viewers; no kill was reported');
  expect(screen.getByRole('status')).not.toHaveTextContent('seat stopped');
});
