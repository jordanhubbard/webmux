import { it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/react';
import { RdpViewer } from '@frontend/components/RdpViewer';

const fixture = vi.hoisted(() => ({ keys: vi.fn(), target: null as HTMLElement | null }));
vi.mock('guacamole-common-js', async importOriginal => {
  const original = await importOriginal<{ default: typeof import('guacamole-common-js') }>();
  const Guacamole = original.default;
  return { default: {
    ...Guacamole,
    WebSocketTunnel: class {},
    Client: class {
      connect() {} disconnect() {}
      sendKeyEvent = fixture.keys;
      getDisplay() { return { getElement: () => document.createElement('canvas') }; }
    },
    Keyboard: class extends Guacamole.Keyboard {
      constructor(target: HTMLElement) { super(target); fixture.target = target; }
    },
  } };
});

it('scopes real Guacamole keyboard listeners to the viewer and releases keys on blur', () => {
  const view = render(<><input aria-label="Other input" /><RdpViewer sessionId="r1" mode="fullscreen" /></>);
  expect(fixture.target).not.toBe(document);
  const input = view.getByLabelText('Other input');
  const event = new KeyboardEvent('keydown', { key: 'a', keyCode: 65, bubbles: true, cancelable: true });
  expect(input.dispatchEvent(event)).toBe(true);
  expect(fixture.keys).not.toHaveBeenCalled();
  const target = fixture.target!;
  fireEvent.keyDown(target, { key: 'Control', keyCode: 17 });
  fireEvent.blur(target);
  expect(fixture.keys).toHaveBeenCalledWith(0, 0xffe3);
  view.unmount();
  const ordinary = document.createElement('input');
  document.body.append(ordinary);
  expect(ordinary.dispatchEvent(new KeyboardEvent('keydown', { key: 'a', keyCode: 65, bubbles: true, cancelable: true }))).toBe(true);
  ordinary.remove();
});
