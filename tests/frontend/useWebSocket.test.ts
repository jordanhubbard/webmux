import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';

// Mock buildWsUrl
vi.mock('@frontend/utils/api', () => ({
  buildWsUrl: (sessionId: string) => `ws://localhost/api/term/${sessionId}`,
}));

// Minimal WebSocket mock
class MockWebSocket {
  static OPEN = 1;
  static instances: MockWebSocket[] = [];

  url: string;
  readyState = MockWebSocket.OPEN;
  onopen: (() => void) | null = null;
  onclose: ((event: { code: number }) => void) | null = null;
  onerror: ((e: unknown) => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  closeCalled = false;
  closeCode?: number;
  sentMessages: string[] = [];

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  send(data: string) {
    this.sentMessages.push(data);
  }

  close(code?: number) {
    this.closeCalled = true;
    this.closeCode = code;
    this.readyState = 3; // CLOSED
  }

  // Helpers for tests
  simulateOpen() { this.onopen?.(); }
  simulateClose(code = 1006) { this.onclose?.({ code }); }
  simulateMessage(data: object) { this.onmessage?.({ data: JSON.stringify(data) }); }
}

describe('useWebSocket', () => {
  let originalWebSocket: typeof globalThis.WebSocket;

  beforeEach(() => {
    vi.useFakeTimers();
    MockWebSocket.instances = [];
    originalWebSocket = globalThis.WebSocket;
    (globalThis as any).WebSocket = MockWebSocket;
  });

  afterEach(() => {
    vi.useRealTimers();
    globalThis.WebSocket = originalWebSocket;
    vi.restoreAllMocks();
  });

  async function importHook() {
    const mod = await import('@frontend/hooks/useWebSocket');
    return mod.useWebSocket;
  }

  it('connects to the correct URL on mount', async () => {
    const useWebSocket = await importHook();
    renderHook(() =>
      useWebSocket({ sessionId: 'abc', onMessage: vi.fn() })
    );
    expect(MockWebSocket.instances.length).toBe(1);
    expect(MockWebSocket.instances[0].url).toBe('ws://localhost/api/term/abc');
  });

  it('calls onOpen when connection opens', async () => {
    const useWebSocket = await importHook();
    const onOpen = vi.fn();
    renderHook(() =>
      useWebSocket({ sessionId: 's1', onMessage: vi.fn(), onOpen })
    );
    act(() => MockWebSocket.instances[0].simulateOpen());
    expect(onOpen).toHaveBeenCalledTimes(1);
  });

  it('calls onMessage with parsed JSON', async () => {
    const useWebSocket = await importHook();
    const onMessage = vi.fn();
    renderHook(() =>
      useWebSocket({ sessionId: 's1', onMessage })
    );
    act(() => {
      MockWebSocket.instances[0].simulateOpen();
      MockWebSocket.instances[0].simulateMessage({ type: 'output', data: 'hello' });
    });
    expect(onMessage).toHaveBeenCalledWith({ type: 'output', data: 'hello' });
  });

  it('ignores malformed messages', async () => {
    const useWebSocket = await importHook();
    const onMessage = vi.fn();
    renderHook(() =>
      useWebSocket({ sessionId: 's1', onMessage })
    );
    act(() => {
      MockWebSocket.instances[0].simulateOpen();
      MockWebSocket.instances[0].onmessage?.({ data: 'not-json{{{' });
    });
    expect(onMessage).not.toHaveBeenCalled();
  });

  it('sends messages when connected', async () => {
    const useWebSocket = await importHook();
    const { result } = renderHook(() =>
      useWebSocket({ sessionId: 's1', onMessage: vi.fn() })
    );
    act(() => MockWebSocket.instances[0].simulateOpen());
    act(() => result.current.send({ type: 'input', data: 'test' }));
    expect(MockWebSocket.instances[0].sentMessages).toEqual([
      JSON.stringify({ type: 'input', data: 'test' }),
    ]);
  });

  it('calls onClose and reconnects with backoff on abnormal close', async () => {
    const useWebSocket = await importHook();
    const onClose = vi.fn();
    renderHook(() =>
      useWebSocket({ sessionId: 's1', onMessage: vi.fn(), onClose })
    );
    const ws1 = MockWebSocket.instances[0];
    act(() => {
      ws1.simulateOpen();
      ws1.simulateClose(1006); // abnormal
    });
    expect(onClose).toHaveBeenCalledTimes(1);

    // Advance past the first reconnect delay (1000ms)
    act(() => { vi.advanceTimersByTime(1000); });
    expect(MockWebSocket.instances.length).toBe(2);
  });

  it('does not reconnect on intentional close (code 1000)', async () => {
    const useWebSocket = await importHook();
    renderHook(() =>
      useWebSocket({ sessionId: 's1', onMessage: vi.fn() })
    );
    act(() => {
      MockWebSocket.instances[0].simulateOpen();
      MockWebSocket.instances[0].simulateClose(1000);
    });
    act(() => { vi.advanceTimersByTime(5000); });
    expect(MockWebSocket.instances.length).toBe(1);
  });

  it('closes with code 1000 on unmount', async () => {
    const useWebSocket = await importHook();
    const { unmount } = renderHook(() =>
      useWebSocket({ sessionId: 's1', onMessage: vi.fn() })
    );
    const ws = MockWebSocket.instances[0];
    unmount();
    expect(ws.closeCalled).toBe(true);
    expect(ws.closeCode).toBe(1000);
  });

  it('close() stops reconnection', async () => {
    const useWebSocket = await importHook();
    const { result } = renderHook(() =>
      useWebSocket({ sessionId: 's1', onMessage: vi.fn() })
    );
    act(() => MockWebSocket.instances[0].simulateOpen());
    act(() => result.current.close());
    expect(MockWebSocket.instances[0].closeCalled).toBe(true);

    // Simulate an abnormal close after manual close — should not reconnect
    act(() => { vi.advanceTimersByTime(5000); });
    expect(MockWebSocket.instances.length).toBe(1);
  });

  it('uses exponential backoff for reconnects', async () => {
    const useWebSocket = await importHook();
    renderHook(() =>
      useWebSocket({ sessionId: 's1', onMessage: vi.fn() })
    );

    // First disconnect → 1s delay
    act(() => {
      MockWebSocket.instances[0].simulateOpen();
      MockWebSocket.instances[0].simulateClose(1006);
    });
    act(() => { vi.advanceTimersByTime(999); });
    expect(MockWebSocket.instances.length).toBe(1);
    act(() => { vi.advanceTimersByTime(1); });
    expect(MockWebSocket.instances.length).toBe(2);

    // Second disconnect → 2s delay
    act(() => { MockWebSocket.instances[1].simulateClose(1006); });
    act(() => { vi.advanceTimersByTime(1999); });
    expect(MockWebSocket.instances.length).toBe(2);
    act(() => { vi.advanceTimersByTime(1); });
    expect(MockWebSocket.instances.length).toBe(3);
  });
});
