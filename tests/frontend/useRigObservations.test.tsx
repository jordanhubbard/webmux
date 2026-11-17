import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { useRigObservations } from '@frontend/utils/useRigObservations';
import { api } from '@frontend/utils/api';
import type { CoordinationStatus } from '@frontend/components/CoordinationDialog';
vi.mock('@frontend/utils/api', () => ({ api: { getRigObservations: vi.fn(), getRigChanges: vi.fn() } }));
const status = { seats: [{ id: 'canonical' }] } as CoordinationStatus;
beforeEach(() => { vi.mocked(api.getRigChanges).mockReset().mockResolvedValue({ contract: 'webmux-rig-changes/v1', available: false, changed: false }); vi.useFakeTimers(); vi.mocked(api.getRigObservations).mockReset().mockResolvedValue(status); });
afterEach(() => vi.useRealTimers());
it('does not discover coordinator inventories for empty, ordinary, or unauthorized workspaces', async () => {
  renderHook(() => useRigObservations(false));
  await act(() => vi.advanceTimersByTimeAsync(30001));
  expect(api.getRigObservations).not.toHaveBeenCalled();
  expect(api.getRigChanges).not.toHaveBeenCalled();
});
it('uses one Rig-only read, clears failed observations and aborts when disabled', async () => {
  const view = renderHook(({ enabled }) => useRigObservations(enabled), { initialProps: { enabled: true } });
  await act(() => Promise.resolve());
  expect(api.getRigObservations).toHaveBeenCalledWith(expect.any(AbortSignal));
  expect(view.result.current).toEqual(status.seats);
  vi.mocked(api.getRigObservations).mockRejectedValueOnce(new Error('unavailable'));
  await act(() => vi.advanceTimersByTimeAsync(10000));
  expect(view.result.current).toEqual([]);
  const signal = vi.mocked(api.getRigObservations).mock.calls[1][0];
  view.rerender({ enabled: false });
  expect(signal?.aborted).toBe(true);
});
it('expires even when discovery hangs, avoids overlapping reads, and discards late results after unmount', async () => {
  const view = renderHook(() => useRigObservations(true));
  await act(() => Promise.resolve());
  let resolve!: (value: CoordinationStatus) => void;
  vi.mocked(api.getRigObservations).mockImplementationOnce(() => new Promise(done => { resolve = done; }));
  await act(() => vi.advanceTimersByTimeAsync(10000));
  const signal = vi.mocked(api.getRigObservations).mock.calls[1][0];
  await act(() => vi.advanceTimersByTimeAsync(20001));
  expect(view.result.current).toEqual([]);
  expect(api.getRigObservations).toHaveBeenCalledTimes(2);
  expect(signal?.aborted).toBe(true);
  view.unmount();
  await act(() => { resolve(status); return Promise.resolve(); });
});
it('treats partial provider errors as unknown', async () => {
  vi.mocked(api.getRigObservations).mockResolvedValue({ ...status, errors: { rig: 'coordinator unavailable' } });
  const view = renderHook(() => useRigObservations(true));
  await act(() => Promise.resolve());
  expect(view.result.current).toEqual([]);
});

it('uses notifications only to invalidate and immediately re-read canonical inventory', async () => {
  let notify!: (value: { contract: string; available: boolean; changed: boolean }) => void;
  vi.mocked(api.getRigChanges).mockImplementationOnce(() => new Promise(done => { notify = done; }));
  const view = renderHook(() => useRigObservations(true));
  await act(() => Promise.resolve());
  const canonical = { seats: [{ id: 'new-canonical' }] } as CoordinationStatus;
  vi.mocked(api.getRigObservations).mockResolvedValue(canonical);
  await act(() => { notify({ contract: 'webmux-rig-changes/v1', available: true, changed: true }); return Promise.resolve(); });
  expect(api.getRigObservations).toHaveBeenCalledTimes(2);
  expect(view.result.current).toEqual(canonical.seats);
  await act(() => vi.advanceTimersByTimeAsync(999));
  expect(api.getRigChanges).toHaveBeenCalledTimes(1);
  await act(() => vi.advanceTimersByTimeAsync(1));
  expect(api.getRigChanges).toHaveBeenCalledTimes(2);
  view.unmount();
});

it('coalesces notifications during inventory reads and cancels subscriptions on disable', async () => {
  let complete!: (value: CoordinationStatus) => void;
  vi.mocked(api.getRigObservations).mockImplementationOnce(() => new Promise(done => { complete = done; }));
  vi.mocked(api.getRigChanges).mockResolvedValueOnce({ contract: 'webmux-rig-changes/v1', available: true, changed: true });
  const view = renderHook(({ enabled }) => useRigObservations(enabled), { initialProps: { enabled: true } });
  await act(() => Promise.resolve());
  expect(api.getRigObservations).toHaveBeenCalledTimes(1);
  await act(() => { complete(status); return Promise.resolve(); });
  expect(api.getRigObservations).toHaveBeenCalledTimes(2);
  const signal = vi.mocked(api.getRigChanges).mock.calls[0][0];
  view.rerender({ enabled: false });
  expect(signal?.aborted).toBe(true);
  await act(() => vi.advanceTimersByTimeAsync(30001));
  expect(api.getRigChanges).toHaveBeenCalledTimes(1);
  expect(view.result.current).toEqual([]);
});

it('bounds hung watches and keeps canonical polling when notifications are unavailable', async () => {
  vi.mocked(api.getRigChanges).mockImplementationOnce(() => new Promise(() => {}));
  const view = renderHook(() => useRigObservations(true));
  await act(() => vi.advanceTimersByTimeAsync(30001));
  expect(vi.mocked(api.getRigChanges).mock.calls[0][0]?.aborted).toBe(true);
  expect(api.getRigChanges).toHaveBeenCalledTimes(1);
  expect(api.getRigObservations).toHaveBeenCalledTimes(4);
  view.unmount();
});
