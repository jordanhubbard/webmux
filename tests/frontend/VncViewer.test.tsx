import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, cleanup, render, screen } from '@testing-library/react';
import { VncViewer } from '@frontend/components/VncViewer';

const fixture = vi.hoisted(() => ({ clients: [] as Array<{
  options: { credentials: { username?: string; password?: string } };
  events: Record<string, (event: { detail: { types: string[] } }) => void>;
  sendCredentials: ReturnType<typeof vi.fn>;
  disconnect: ReturnType<typeof vi.fn>;
}> }));

vi.mock('@novnc/novnc', () => ({ default: class {
  events: Record<string, (event: { detail: { types: string[] } }) => void> = {};
  sendCredentials = vi.fn();
  disconnect = vi.fn();
  constructor(_target: Element, _url: string, public options: { credentials: { username?: string; password?: string } }) {
    fixture.clients.push(this);
  }
  addEventListener(name: string, handler: (event: { detail: { types: string[] } }) => void) { this.events[name] = handler; }
} }));
vi.mock('@frontend/utils/api', () => ({ buildVncWsUrl: () => 'ws://localhost/fixture' }));

beforeEach(() => {
  fixture.clients.length = 0;
  vi.stubGlobal('isSecureContext', true);
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it.each(['thumbnail', 'fullscreen'] as const)('passes the entered username during initial Apple authentication and credential requests in %s', mode => {
  const view = render(<VncViewer sessionId="v1" mode={mode} vncUsername="mac-user" vncPassword="first" />);
  const client = fixture.clients[0];
  expect(client.options.credentials).toEqual({ username: 'mac-user', password: 'first' });
  view.rerender(<VncViewer sessionId="v1" mode={mode} vncUsername="mac-user" vncPassword="updated" />);
  act(() => client.events.credentialsrequired({ detail: { types: ['username', 'password'] } }));
  expect(client.sendCredentials).toHaveBeenCalledWith({ username: 'mac-user', password: 'updated' });
  expect(fixture.clients).toHaveLength(1);
});

it('preserves password-only VNC and reports a missing Apple username without retrying forever', () => {
  render(<VncViewer sessionId="v1" mode="thumbnail" vncPassword="vnc-secret" />);
  const client = fixture.clients[0];
  act(() => client.events.credentialsrequired({ detail: { types: ['password'] } }));
  expect(client.sendCredentials).toHaveBeenCalledWith({ username: undefined, password: 'vnc-secret' });
  client.sendCredentials.mockClear();
  act(() => client.events.credentialsrequired({ detail: { types: ['username', 'password'] } }));
  expect(screen.getByRole('alert').textContent).toContain('requires a VNC username');
  expect(client.sendCredentials).not.toHaveBeenCalled();
  expect(client.disconnect).toHaveBeenCalled();
});

it('explains HTTPS or localhost before attempting an insecure browser connection', () => {
  vi.stubGlobal('isSecureContext', false);
  const onStateChange = vi.fn();
  render(<VncViewer sessionId="v1" mode="fullscreen" onStateChange={onStateChange} />);
  expect(screen.getByRole('alert').textContent).toContain('HTTPS or localhost');
  expect(fixture.clients).toHaveLength(0);
  expect(onStateChange).toHaveBeenCalledWith('error');
});
