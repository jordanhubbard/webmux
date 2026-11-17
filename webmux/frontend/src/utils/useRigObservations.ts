import { useEffect, useState } from 'react';
import { api } from './api';
import type { CoordinationSeat } from '../components/CoordinationDialog';

// One bounded, non-overlapping read for the workspace; never probes empty or
// ordinary-terminal workspaces. Expiry applies even if a request hangs.
export function useRigObservations(enabled: boolean): CoordinationSeat[] {
  const [seats, setSeats] = useState<CoordinationSeat[]>([]);
  useEffect(() => {
    if (!enabled) { setSeats([]); return; }
    let disposed = false;
    let pending = false;
    let invalidated = false;
    let changeTimer: ReturnType<typeof setTimeout> | undefined;
    let changeController: AbortController | undefined;
    let changeDeadline: ReturnType<typeof setTimeout> | undefined;
    let expiry: ReturnType<typeof setTimeout> | undefined;
    let controller: AbortController | undefined;
    const refresh = async () => {
      if (disposed || pending) return;
      pending = true;
      controller = new AbortController();
      const timeout = setTimeout(() => controller?.abort(), 20000);
      try {
        const result = await api.getRigObservations(controller.signal);
        if (disposed || controller.signal.aborted) return;
        // The endpoint can return partial provider errors with HTTP 200.
        setSeats(result.errors?.rig ? [] : result.seats || []);
        clearTimeout(expiry);
        expiry = setTimeout(() => setSeats([]), 30000);
      } catch {
        if (!disposed) setSeats([]);
      } finally {
        clearTimeout(timeout);
        pending = false;
        if (invalidated && !disposed) { invalidated = false; void refresh(); }
      }
    };
    const invalidate = () => {
      if (pending) invalidated = true;
      else void refresh();
    };
    const watch = async () => {
      if (disposed) return;
      changeController = new AbortController();
      changeDeadline = setTimeout(() => changeController?.abort(), 30000);
      let delay = 30000;
      try {
        const result = await api.getRigChanges(changeController.signal);
        if (disposed || changeController.signal.aborted) return;
        if (result.contract !== 'webmux-rig-changes/v1') return;
        if (result.available) delay = 1000;
        if (result.available && result.changed) invalidate();
      } catch { /* Periodic canonical reads remain the fallback. */ }
      finally {
        clearTimeout(changeDeadline);
        if (!disposed) changeTimer = setTimeout(() => { void watch(); }, delay);
      }
    };
    void watch();
    void refresh();
    const timer = setInterval(() => { void refresh(); }, 10000);
    window.addEventListener('webmux-sessions-changed', invalidate);
    return () => {
      disposed = true;
      clearInterval(timer);
      clearTimeout(changeTimer);
      clearTimeout(changeDeadline);
      changeController?.abort();
      clearTimeout(expiry);
      controller?.abort();
      window.removeEventListener('webmux-sessions-changed', invalidate);
    };
  }, [enabled]);
  return enabled ? seats : [];
}
