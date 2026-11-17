import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { useRigObservations } from '@frontend/utils/useRigObservations';
import { api } from '@frontend/utils/api';
import type { CoordinationStatus } from '@frontend/components/CoordinationDialog';
vi.mock('@frontend/utils/api', () => ({ api: { getRigObservations: vi.fn() } }));
const status = { seats: [{ id: 'canonical' }] } as CoordinationStatus;
beforeEach(() => { vi.useFakeTimers(); vi.mocked(api.getRigObservations).mockReset().mockResolvedValue(status); });
afterEach(() => vi.useRealTimers());
it('does not discover coordinator inventories for empty, ordinary, or unauthorized workspaces', async () => {
  renderHook(() => useRigObservations(false));
  await act(() => vi.advanceTimersByTimeAsync(30001));
  expect(api.getRigObservations).not.toHaveBeenCalled();
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
