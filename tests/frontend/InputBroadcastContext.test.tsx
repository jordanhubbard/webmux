import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { InputBroadcastProvider, useInputBroadcast } from '@frontend/contexts/InputBroadcastContext';

// Test component that exposes context state and actions
function TestHarness({ sessionIds }: { sessionIds: string[] }) {
  const { broadcastMode, setBroadcastMode, focusedSessionId, setFocusedSessionId, registerSend, unregisterSend, routeInput } = useInputBroadcast();

  return (
    <div>
      <div data-testid="broadcast-mode">{String(broadcastMode)}</div>
      <div data-testid="focused-session">{focusedSessionId ?? 'none'}</div>
      <button data-testid="toggle-broadcast" onClick={() => setBroadcastMode(!broadcastMode)}>toggle</button>
      {sessionIds.map(id => (
        <button key={id} data-testid={`focus-${id}`} onClick={() => setFocusedSessionId(id)}>focus {id}</button>
      ))}
      <button data-testid="route-input" onClick={() => routeInput(focusedSessionId ?? sessionIds[0], 'hello')}>send</button>
    </div>
  );
}

describe('InputBroadcastContext', () => {
  it('defaults to broadcast mode off and no focused session', () => {
    render(
      <InputBroadcastProvider>
        <TestHarness sessionIds={['s1']} />
      </InputBroadcastProvider>,
    );
    expect(screen.getByTestId('broadcast-mode').textContent).toBe('false');
    expect(screen.getByTestId('focused-session').textContent).toBe('none');
  });

  it('toggles broadcast mode', () => {
    render(
      <InputBroadcastProvider>
        <TestHarness sessionIds={['s1']} />
      </InputBroadcastProvider>,
    );
    fireEvent.click(screen.getByTestId('toggle-broadcast'));
    expect(screen.getByTestId('broadcast-mode').textContent).toBe('true');
    fireEvent.click(screen.getByTestId('toggle-broadcast'));
    expect(screen.getByTestId('broadcast-mode').textContent).toBe('false');
  });

  it('sets focused session id', () => {
    render(
      <InputBroadcastProvider>
        <TestHarness sessionIds={['s1', 's2']} />
      </InputBroadcastProvider>,
    );
    fireEvent.click(screen.getByTestId('focus-s2'));
    expect(screen.getByTestId('focused-session').textContent).toBe('s2');
  });

  it('routes input only to focused session when broadcast is off', () => {
    const send1 = vi.fn();
    const send2 = vi.fn();

    function Setup() {
      const { registerSend, routeInput, setFocusedSessionId } = useInputBroadcast();
      return (
        <div>
          <button data-testid="register" onClick={() => { registerSend('s1', send1); registerSend('s2', send2); }}>reg</button>
          <button data-testid="focus-s1" onClick={() => setFocusedSessionId('s1')}>focus</button>
          <button data-testid="send" onClick={() => routeInput('s1', 'test')}>send</button>
        </div>
      );
    }

    render(
      <InputBroadcastProvider>
        <Setup />
      </InputBroadcastProvider>,
    );

    fireEvent.click(screen.getByTestId('register'));
    fireEvent.click(screen.getByTestId('focus-s1'));
    fireEvent.click(screen.getByTestId('send'));

    expect(send1).toHaveBeenCalledWith('test');
    expect(send2).not.toHaveBeenCalled();
  });

  it('routes input to all sessions when broadcast is on', () => {
    const send1 = vi.fn();
    const send2 = vi.fn();

    function Setup() {
      const { registerSend, routeInput, setBroadcastMode } = useInputBroadcast();
      return (
        <div>
          <button data-testid="register" onClick={() => { registerSend('s1', send1); registerSend('s2', send2); }}>reg</button>
          <button data-testid="broadcast-on" onClick={() => setBroadcastMode(true)}>on</button>
          <button data-testid="send" onClick={() => routeInput('s1', 'test')}>send</button>
        </div>
      );
    }

    render(
      <InputBroadcastProvider>
        <Setup />
      </InputBroadcastProvider>,
    );

    fireEvent.click(screen.getByTestId('register'));
    fireEvent.click(screen.getByTestId('broadcast-on'));
    fireEvent.click(screen.getByTestId('send'));

    expect(send1).toHaveBeenCalledWith('test');
    expect(send2).toHaveBeenCalledWith('test');
  });

  it('unregisterSend stops routing to that session', () => {
    const send1 = vi.fn();
    const send2 = vi.fn();

    function Setup() {
      const { registerSend, unregisterSend, routeInput, setBroadcastMode } = useInputBroadcast();
      return (
        <div>
          <button data-testid="register" onClick={() => { registerSend('s1', send1); registerSend('s2', send2); }}>reg</button>
          <button data-testid="unregister" onClick={() => unregisterSend('s2')}>unreg</button>
          <button data-testid="broadcast-on" onClick={() => setBroadcastMode(true)}>on</button>
          <button data-testid="send" onClick={() => routeInput('s1', 'test')}>send</button>
        </div>
      );
    }

    render(
      <InputBroadcastProvider>
        <Setup />
      </InputBroadcastProvider>,
    );

    fireEvent.click(screen.getByTestId('register'));
    fireEvent.click(screen.getByTestId('unregister'));
    fireEvent.click(screen.getByTestId('broadcast-on'));
    fireEvent.click(screen.getByTestId('send'));

    expect(send1).toHaveBeenCalledWith('test');
    expect(send2).not.toHaveBeenCalled();
  });

  it('throws when used outside provider', () => {
    function Bad() {
      useInputBroadcast();
      return null;
    }
    expect(() => render(<Bad />)).toThrow('useInputBroadcast must be used within InputBroadcastProvider');
  });
});
