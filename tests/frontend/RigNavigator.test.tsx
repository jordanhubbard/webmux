import { beforeEach, expect, it, vi } from 'vitest';
import { act, render, screen, fireEvent, waitFor } from '@testing-library/react';
import { RigNavigator } from '@frontend/components/RigNavigator';
import type { CoordinationStatus } from '@frontend/components/CoordinationDialog';
import { api } from '@frontend/utils/api';

vi.mock('@frontend/utils/api', () => ({ api: { getCoordination: vi.fn() } }));
const status: CoordinationStatus = {
  contract: 'webmux-coordination/v1', defaultLaunchProvider: 'rig', defaultView: 'all', enabledProviders: ['rig'], providers: [], messages: [], errors: {},
  projects: [{ provider: 'rig', id: 'project', name: 'remote project', host: 'worker.example', state: 'running', historical: false, seatCount: 1, runningCount: 1, contexts: [] }],
  seats: [{ provider: 'rig', projectId: 'project', projectName: 'remote project', id: 'host-rig/client/seat', name: 'worker', host: 'worker.example', runtime: 'codex', state: 'running', observationState: 'current', transportState: 'connected', openable: true, managed: true }],
};
beforeEach(() => { vi.mocked(api.getCoordination).mockReset().mockResolvedValue(status); });

it('keeps an offline host distinct from the last observed running seat in grid navigation', async () => {
  vi.mocked(api.getCoordination).mockResolvedValue({ ...status, seats: [{ ...status.seats[0], observationState: 'offline', transportState: 'offline', openable: false }] });
  const open = vi.fn();
  render(<RigNavigator onOpen={open} onManage={() => {}} />);
  fireEvent.click(screen.getByText('Coordinated sessions'));
  const seat = await screen.findByRole('button', { name: 'worker · worker.example · last observed running; offline · Rig' });
  expect(seat).toBeDisabled();
  expect(seat).toHaveAttribute('title', expect.stringContaining('attachment offline'));
  expect(open).not.toHaveBeenCalled();
});

it('does not discover inventories until the owner opens the navigator', async () => {
  const open = vi.fn().mockResolvedValue(undefined);
  render(<RigNavigator onOpen={open} onManage={() => {}} />);
  expect(api.getCoordination).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText('Coordinated sessions'));
  fireEvent.click(await screen.findByRole('button', { name: 'worker · worker.example · running · Rig' }));
  await waitFor(() => expect(open).toHaveBeenCalledExactlyOnceWith('rig', 'host-rig/client/seat', 'worker'));
});

it('aborts a hidden-pane request and ignores its late reply before making a fresh request on return', async () => {
  let resolve!: (value: CoordinationStatus) => void;
  vi.mocked(api.getCoordination).mockImplementation(() => new Promise(done => { resolve = done; }));
  const props = { onOpen: vi.fn(), onManage: vi.fn() };
  const view = render(<RigNavigator {...props} />);
  fireEvent.click(screen.getByText('Coordinated sessions'));
  await waitFor(() => expect(api.getCoordination).toHaveBeenCalledTimes(1));
  const signal = vi.mocked(api.getCoordination).mock.calls[0][1]!;
  view.rerender(<RigNavigator {...props} active={false} />);
  expect(signal.aborted).toBe(true);
  await act(async () => { resolve(status); });
  expect(screen.queryByRole('button', { name: 'worker · worker.example · running · Rig' })).not.toBeInTheDocument();
  view.rerender(<RigNavigator {...props} active />);
  expect(api.getCoordination).toHaveBeenCalledTimes(2);
  await act(async () => { resolve(status); });
  expect(await screen.findByRole('button', { name: 'worker · worker.example · running · Rig' })).toBeEnabled();
});

it('removes stale seat actions when a refreshed inventory is unavailable', async () => {
  const open = vi.fn();
  render(<RigNavigator onOpen={open} onManage={() => {}} />);
  fireEvent.click(screen.getByText('Coordinated sessions'));
  await screen.findByRole('button', { name: 'worker · worker.example · running · Rig' });
  vi.mocked(api.getCoordination).mockRejectedValueOnce(new Error('unavailable'));
  fireEvent(window, new CustomEvent('webmux-sessions-changed'));
  await screen.findByText('Coordination status unavailable: Error: unavailable');
  expect(screen.queryByRole('button', { name: 'worker · worker.example · running · Rig' })).not.toBeInTheDocument();
  expect(open).not.toHaveBeenCalled();
});
