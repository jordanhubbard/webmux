import { beforeEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MacPermissions } from '@frontend/components/MacPermissions';
import { api, type MacSetup } from '@frontend/utils/api';

vi.mock('@frontend/utils/api', () => ({ api: { getMacSetup: vi.fn(), macSetupAction: vi.fn() } }));
let setup: MacSetup;
beforeEach(() => {
  vi.resetAllMocks();
  setup = { skipped: false, status: { instance: 'first', supported: true, hostname: 'puck', executable: '/Applications/WebMux.app/Contents/MacOS/webmux', permission_target: '/Applications/WebMux.app', can_open_settings: false, local: { state: 'unchecked' }, ssh: { state: 'unchecked' } } };
  vi.mocked(api.getMacSetup).mockImplementation(async () => structuredClone(setup));
  vi.mocked(api.macSetupAction).mockResolvedValue({ ok: true });
});
it('guides first run on the server Mac without automatically requesting access', async () => {
  render(<MacPermissions />);
  expect(await screen.findByRole('dialog', { name: 'Mac permissions' })).toBeDefined();
  expect(screen.getByText('/Applications/WebMux.app')).toBeDefined();
  expect(screen.getByRole('button', { name: 'Open Privacy settings' })).toBeDisabled();
  expect(api.macSetupAction).not.toHaveBeenCalled();
});
it('keeps separate local and SSH results and explains a timed-out permission request', async () => {
  render(<MacPermissions />);
  await screen.findByRole('dialog');
  setup.status.local = { state: 'waiting', detail: 'Approve the prompt on puck.' };
  fireEvent.click(screen.getByRole('button', { name: 'Check local access' }));
  expect(await screen.findByText('Still waiting for macOS')).toBeDefined();
  expect(screen.getByText('Approve the prompt on puck.')).toBeDefined();
  expect(api.macSetupAction).toHaveBeenCalledWith('check', 'local');
  expect(screen.getByText('Not checked')).toBeDefined();
  setup.status.ssh = { state: 'denied' };
  fireEvent.click(screen.getByRole('button', { name: 'Check localhost SSH access' }));
  expect(await screen.findByText('Permission denied')).toBeDefined();
});
it('persists skipping while leaving an actionable banner to reopen setup', async () => {
  render(<MacPermissions />);
  fireEvent.click(await screen.findByRole('button', { name: 'Skip for now' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  expect(api.macSetupAction).toHaveBeenCalledWith('skip', undefined);
  fireEvent.click(screen.getByRole('button', { name: 'Mac permissions' }));
  expect(screen.getByRole('dialog')).toBeDefined();
});
it('does not show first-run setup again after skipping or on non-Mac hosts', async () => {
  setup.skipped = true;
  const { unmount } = render(<MacPermissions />);
  await screen.findByRole('button', { name: 'Mac permissions' });
  expect(screen.queryByRole('dialog')).toBeNull();
  unmount();
  setup.status.supported = false;
  render(<MacPermissions />);
  await waitFor(() => expect(api.getMacSetup).toHaveBeenCalledTimes(2));
  expect(screen.queryByRole('button', { name: 'Mac permissions' })).toBeNull();
});
it('shows errors and retains the wizard for retry', async () => {
  vi.mocked(api.macSetupAction).mockRejectedValue(new Error('Could not open the Mac desktop.'));
  setup.status.can_open_settings = true;
  render(<MacPermissions />);
  fireEvent.click(await screen.findByRole('button', { name: 'Open Privacy settings' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not open the Mac desktop.');
  expect(screen.getByRole('button', { name: 'Check local access' })).toBeEnabled();
});
it('confirms a new process instance before reporting restart success', async () => {
  render(<MacPermissions />);
  fireEvent.click(await screen.findByRole('button', { name: 'Restart WebMux and reconnect' }));
  await screen.findByRole('button', { name: 'Restarting WebMux…' });
  expect(screen.queryByText(/WebMux restarted\./)).toBeNull();
  setup.status.instance = 'second';
  expect(await screen.findByText(/WebMux restarted\./, {}, { timeout: 2500 })).toBeDefined();
});
