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
      }
    };
    void refresh();
    const timer = setInterval(() => { void refresh(); }, 10000);
    window.addEventListener('webmux-sessions-changed', refresh);
    return () => {
      disposed = true;
      clearInterval(timer);
      clearTimeout(expiry);
      controller?.abort();
      window.removeEventListener('webmux-sessions-changed', refresh);
    };
  }, [enabled]);
  return enabled ? seats : [];
}
