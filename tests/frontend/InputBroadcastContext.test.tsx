import type { PropsWithChildren } from 'react';
import { act, cleanup, renderHook, render, screen, fireEvent } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { InputBroadcastProvider, useInputBroadcast } from '@frontend/contexts/InputBroadcastContext';

afterEach(cleanup);
const wrapper = ({ children }: PropsWithChildren) => <InputBroadcastProvider>{children}</InputBroadcastProvider>;
function setup() {
  const view = renderHook(() => useInputBroadcast(), { wrapper });
  const a = vi.fn(), b = vi.fn(), c = vi.fn();
  act(() => {
    view.result.current.registerSend('a', a);
    view.result.current.registerSend('b', b);
    view.result.current.setBroadcastMode(true);
  });
  return { ...view, a, b, c };
}

it('does not add a newly registered terminal to an explicit recipient selection', () => {
  const { result, a, b, c } = setup();
  act(() => result.current.setBroadcastRecipients(['a', 'b'], ['a']));
  act(() => result.current.registerSend('c', c));
  act(() => result.current.routeInput('a', 'selected'));
  expect(a).toHaveBeenCalledWith('selected');
  expect(b).not.toHaveBeenCalled();
  expect(c).not.toHaveBeenCalled();
});

it('keeps None silent when another terminal registers', () => {
  const { result, a, b, c } = setup();
  act(() => result.current.setBroadcastRecipients(['a', 'b'], []));
  act(() => result.current.registerSend('c', c));
  act(() => result.current.routeInput('a', 'not-for-anyone'));
  expect(a).not.toHaveBeenCalled(); expect(b).not.toHaveBeenCalled(); expect(c).not.toHaveBeenCalled();
});

it('applies a recipient choice before routing in the same event', () => {
  const { result, a, b } = setup();
  act(() => {
    result.current.setBroadcastRecipients(['a', 'b'], ['a']);
    result.current.routeInput('a', 'new-intent');
  });
  expect(a).toHaveBeenCalledWith('new-intent'); expect(b).not.toHaveBeenCalled();
});

it('stops broadcasting before routing in the mode-off event', () => {
  const { result, a, b } = setup();
  act(() => {
    result.current.setBroadcastMode(false);
    result.current.routeInput('a', 'ordinary');
  });
  expect(a).toHaveBeenCalledWith('ordinary'); expect(b).not.toHaveBeenCalled();
});

it('retains the default broadcast and existing exclusion behavior', () => {
  const { result, a, b } = setup();
  act(() => result.current.routeInput('a', 'both'));
  expect(a).toHaveBeenCalledWith('both'); expect(b).toHaveBeenCalledWith('both');
  act(() => result.current.toggleBroadcastExclude('b'));
  act(() => result.current.routeInput('a', 'a-only'));
  expect(a).toHaveBeenCalledWith('a-only'); expect(b.mock.calls).toEqual([['both']]);
});

it('permits explicit inclusion of a terminal that opened after selection', () => {
  const { result, a, b, c } = setup();
  act(() => result.current.setBroadcastRecipients(['a', 'b'], ['a']));
  act(() => result.current.registerSend('c', c));
  act(() => result.current.toggleBroadcastExclude('c'));
  act(() => result.current.routeInput('a', 'chosen-later'));
  expect(a).toHaveBeenCalledWith('chosen-later'); expect(c).toHaveBeenCalledWith('chosen-later'); expect(b).not.toHaveBeenCalled();
});

it('retains selection when the same viewer re-registers with a new sender', () => {
  const { result, a, b, c } = setup();
  act(() => result.current.setBroadcastRecipients(['a', 'b'], ['a']));
  act(() => { result.current.unregisterSend('a'); result.current.registerSend('a', c); });
  act(() => result.current.routeInput('a', 'reconnected'));
  expect(c).toHaveBeenCalledWith('reconnected'); expect(a).not.toHaveBeenCalled(); expect(b).not.toHaveBeenCalled();
});

it('routes ordinary input to its source regardless of the retained broadcast selection', () => {
  const { result, a, b } = setup();
  act(() => result.current.setBroadcastRecipients(['a', 'b'], ['a']));
  act(() => result.current.setBroadcastMode(false));
  act(() => result.current.routeInput('b', 'ordinary-b'));
  expect(b).toHaveBeenCalledWith('ordinary-b'); expect(a).not.toHaveBeenCalled();
});

it('cannot preselect an ID outside the displayed selection inventory', () => {
  const { result, a, b, c } = setup();
  act(() => result.current.setBroadcastRecipients(['a', 'b'], ['a', 'c']));
  act(() => result.current.registerSend('c', c));
  act(() => result.current.routeInput('a', 'known-only'));
  expect(a).toHaveBeenCalledWith('known-only'); expect(b).not.toHaveBeenCalled(); expect(c).not.toHaveBeenCalled();
});

it('keeps a manual recipient exclusion fixed when another terminal registers', () => {
  const { result, a, b, c } = setup();
  act(() => result.current.toggleBroadcastExclude('b'));
  act(() => result.current.registerSend('c', c));
  act(() => result.current.routeInput('a', 'manual-selection'));
  expect(a).toHaveBeenCalledWith('manual-selection'); expect(b).not.toHaveBeenCalled(); expect(c).not.toHaveBeenCalled();
});


it('does not send to an already registered terminal outside the displayed selection inventory', () => {
  const { result, a, b } = setup();
  act(() => result.current.setBroadcastRecipients(['a'], ['a']));
  act(() => result.current.routeInput('a', 'visible-only'));
  expect(a).toHaveBeenCalledWith('visible-only'); expect(b).not.toHaveBeenCalled();
});

// Test component that exposes context state and actions
function TestHarness({ sessionIds }: { sessionIds: string[] }) {
  const { broadcastMode, setBroadcastMode, focusedSessionId, setFocusedSessionId, routeInput } = useInputBroadcast();

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

it('keeps default broadcast dynamic while an explicit All choice stays fixed', () => {
  const { result, a, b, c } = setup();
  const later = vi.fn();
  act(() => result.current.registerSend('c', c));
  act(() => result.current.routeInput('a', 'default-all'));
  for (const send of [a, b, c]) expect(send).toHaveBeenCalledWith('default-all');
  act(() => result.current.setBroadcastRecipients(['a', 'b', 'c'], ['a', 'b', 'c']));
  act(() => result.current.registerSend('later', later));
  act(() => result.current.routeInput('a', 'chosen-all'));
  for (const send of [a, b, c]) expect(send).toHaveBeenCalledWith('chosen-all');
  expect(later).not.toHaveBeenCalled();
});
