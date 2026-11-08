import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { VncConnectionDialog } from '@frontend/components/VncConnectionDialog';

vi.mock('@frontend/utils/api', () => ({ api: { getHosts: vi.fn().mockResolvedValue([
  { id: 'mac', hostname: 'mac.example', vnc_enabled: true, vnc_port: 5900, username: 'ssh-user' },
]) } }));
afterEach(cleanup);

it('sends an optional trimmed VNC username for manual and saved-host connections', async () => {
  const onConnect = vi.fn().mockResolvedValue(undefined);
  render(<VncConnectionDialog onConnect={onConnect} onClose={vi.fn()} />);
  fireEvent.change(screen.getByLabelText('VNC Username (optional)'), { target: { value: ' polar ' } });
  fireEvent.change(screen.getByPlaceholderText('hostname or IP'), { target: { value: '192.168.65.5' } });
  fireEvent.change(screen.getByPlaceholderText('Leave blank if none'), { target: { value: 'unchanged-secret' } });
  fireEvent.click(screen.getByRole('button', { name: /^Connect$/ }));
  await waitFor(() => expect(onConnect).toHaveBeenCalledWith(expect.objectContaining({
    hostname: '192.168.65.5', vnc_username: 'polar',
  }), 'unchanged-secret'));
  const savedHost = await screen.findByTitle('VNC connect to mac.example:5900');
  await waitFor(() => expect(savedHost).not.toBeDisabled());
  fireEvent.click(savedHost);
  await waitFor(() => expect(onConnect).toHaveBeenLastCalledWith(expect.objectContaining({ host_id: 'mac', vnc_username: 'polar' }), 'unchanged-secret'));
});

it('leaves the username unset for password-only VNC', async () => {
  const onConnect = vi.fn().mockResolvedValue(undefined);
  render(<VncConnectionDialog onConnect={onConnect} onClose={vi.fn()} />);
  fireEvent.click(await screen.findByTitle('VNC connect to mac.example:5900'));
  await waitFor(() => expect(onConnect).toHaveBeenCalledWith(expect.objectContaining({ vnc_username: undefined }), ''));
});
