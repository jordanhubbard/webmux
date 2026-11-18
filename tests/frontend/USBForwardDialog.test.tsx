import { beforeEach, describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { USBForwardDialog } from '@frontend/components/USBForwardDialog';
import { api, type USBForwardingLease, type USBForwardingSpec } from '@frontend/utils/api';

vi.mock('@frontend/utils/api', () => ({ api: {
  getUSBForwardings: vi.fn(), createUSBForwarding: vi.fn(), stopUSBForwarding: vi.fn(),
} }));
const makeLease = (overrides: Partial<USBForwardingLease> = {}): USBForwardingLease => ({
  id: 'a'.repeat(32), spec: { host: 'dev@build-host', ssh_port: 2222, export_port: 3240, seconds: 3600 },
  phase: 'pending', created_at: new Date().toISOString(), expires_at: new Date(Date.now() + 300000).toISOString(),
  last_seen: '', remote_port: 0, stop_requested: false, release_confirmed: false, stale: false, ...overrides,
});
beforeEach(() => {
  vi.mocked(api.getUSBForwardings).mockReset().mockResolvedValue([]);
  vi.mocked(api.createUSBForwarding).mockReset().mockImplementation(async (spec: USBForwardingSpec) => ({
    lease: makeLease({ spec }), pair_code: 'b'.repeat(48),
  }));
  vi.mocked(api.stopUSBForwarding).mockReset();
});


describe('USB redirection setup', () => {
  it('copies a local command and explains the native boundary', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    render(<USBForwardDialog onClose={vi.fn()} />);
    expect(screen.getByRole('button', { name: 'Copy local command' })).toBeDisabled();
    fireEvent.change(screen.getByLabelText('Receiving SSH host'), { target: { value: 'dev@build-host' } });
    fireEvent.change(screen.getByLabelText('SSH port'), { target: { value: '2222' } });
    fireEvent.change(screen.getByLabelText('Local exporter port'), { target: { value: '3240' } });
    fireEvent.change(screen.getByLabelText('Lease'), { target: { value: '60' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create local command' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Copy local command' })).toBeEnabled());
    const command = (screen.getByLabelText('USB forwarding command') as HTMLTextAreaElement).value;
    expect(command).toMatch(/^webmux usb-forward --host dev@build-host --ssh-port 2222 --export-port 3240 --duration 3600s --dashboard-link [A-Za-z0-9_-]+$/);
    const link = JSON.parse(atob(command.split('--dashboard-link ')[1].replace(/-/g, '+').replace(/_/g, '/'))) as unknown;
    expect(link).toEqual({ version: 1, server: window.location.origin, id: 'a'.repeat(32), pair_code: 'b'.repeat(48), allow_http: window.location.protocol === 'http:' });
    fireEvent.click(screen.getByRole('button', { name: 'Copy local command' }));
    expect(writeText).toHaveBeenCalledWith(command);
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Command copied'));
    expect(screen.getByText(/Creating a command does not start forwarding/)).toBeInTheDocument();
  });
  it.each(['-oProxyCommand=evil', 'host;whoami', '$(id)', 'user@host & calc', 'host%PATH%', 'host`id`'])('rejects unsafe destination %s', host => {
    render(<USBForwardDialog onClose={vi.fn()} />);
    fireEvent.change(screen.getByLabelText('Receiving SSH host'), { target: { value: host } });
    expect(screen.getByRole('button', { name: 'Copy local command' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Create local command' })).toBeDisabled();
    expect(screen.getByLabelText('USB forwarding command')).toHaveValue('');
  });
  it.each(['0', '1023', '65536', '1e4', '3240;id'])('rejects invalid exporter port %s', port => {
    render(<USBForwardDialog onClose={vi.fn()} />);
    fireEvent.change(screen.getByLabelText('Receiving SSH host'), { target: { value: 'build-host' } });
    fireEvent.change(screen.getByLabelText('Local exporter port'), { target: { value: port } });
    expect(screen.getByRole('button', { name: 'Copy local command' })).toBeDisabled();
  });
  it('keeps the command selectable when clipboard access fails', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } });
    render(<USBForwardDialog onClose={vi.fn()} />);
    fireEvent.change(screen.getByLabelText('Receiving SSH host'), { target: { value: 'build-host' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create local command' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Copy local command' })).toBeEnabled());
    fireEvent.click(screen.getByRole('button', { name: 'Copy local command' }));
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Clipboard unavailable'));
    expect((screen.getByLabelText('USB forwarding command') as HTMLTextAreaElement).value).toContain('--host build-host');
  });

  it('shows transport readiness and waits for the helper to confirm release after Stop', async () => {
    const lease = makeLease({ phase: 'ready', remote_port: 23240 });
    vi.mocked(api.getUSBForwardings).mockResolvedValue([lease]);
    vi.mocked(api.stopUSBForwarding).mockResolvedValue({ ...lease, phase: 'stopping', stop_requested: true });
    render(<USBForwardDialog onClose={vi.fn()} />);
    expect(await screen.findByText(/Transport ready on 127.0.0.1:23240/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Stop forwarding to dev@build-host' }));
    expect(await screen.findByText('Stop requested; waiting for remote release')).toBeInTheDocument();
    expect(screen.queryByText(/Forwarding ended; remote listener released/)).not.toBeInTheDocument();
    expect(api.stopUSBForwarding).toHaveBeenCalledExactlyOnceWith(lease.id);
  });

  it('shows stale status without claiming the device or listener was released', async () => {
    vi.mocked(api.getUSBForwardings).mockResolvedValue([makeLease({ phase: 'ready', remote_port: 23240, stale: true })]);
    render(<USBForwardDialog onClose={vi.fn()} />);
    expect(await screen.findByText(/Status unavailable; the local lease deadline/)).toBeInTheDocument();
    expect(screen.queryByText(/Transport ready on/)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Stop forwarding to dev@build-host' })).toBeEnabled();
  });
});
