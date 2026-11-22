import { StrictMode } from 'react';
import { beforeEach, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { SessionCoordinationPolicy } from '@frontend/components/SessionCoordinationPolicy';
import { api } from '@frontend/utils/api';

vi.mock('@frontend/utils/api', () => ({ api: { getSessionCoordinationPolicy: vi.fn(), setSessionCoordinationPolicy: vi.fn() } }));
const policy = (enabled: boolean) => ({ contract: 'webmux-session-coordination-policy/v1' as const, enabled });
const checkbox = () => screen.getByRole('checkbox', { name: 'Enable automatic session coordination' });
const reload = () => screen.getByRole('button', { name: 'Reload coordination policy' });
beforeEach(() => {
  vi.mocked(api.getSessionCoordinationPolicy).mockReset().mockResolvedValue(policy(true));
  vi.mocked(api.setSessionCoordinationPolicy).mockReset().mockResolvedValue(undefined);
});

it('waits for authoritative readback before displaying a saved change', async () => {
  let finish!: (value: ReturnType<typeof policy>) => void;
  vi.mocked(api.getSessionCoordinationPolicy).mockResolvedValueOnce(policy(true))
    .mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
  render(<SessionCoordinationPolicy />);
  await waitFor(() => expect(checkbox()).toBeEnabled());
  fireEvent.click(checkbox());
  await waitFor(() => expect(api.getSessionCoordinationPolicy).toHaveBeenCalledTimes(2));
  expect(checkbox()).toBeChecked(); expect(checkbox()).toBeDisabled();
  await act(async () => { finish(policy(false)); });
  expect(checkbox()).not.toBeChecked(); expect(checkbox()).toBeEnabled();
  expect(screen.getByRole('status')).toHaveTextContent('disabled on this server');
  expect(api.setSessionCoordinationPolicy).toHaveBeenCalledExactlyOnceWith(false, expect.any(AbortSignal));
});

it('represents a failed initial read as unknown and reloads without writing', async () => {
  vi.mocked(api.getSessionCoordinationPolicy).mockRejectedValueOnce(new Error('unavailable')).mockResolvedValue(policy(false));
  render(<SessionCoordinationPolicy />);
  expect(await screen.findByRole('alert')).toHaveTextContent('Cannot load');
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
  fireEvent.click(reload());
  await waitFor(() => expect(checkbox()).toBeEnabled());
  expect(checkbox()).not.toBeChecked(); expect(api.setSessionCoordinationPolicy).not.toHaveBeenCalled();
});

it.each(['write reply', 'readback'])('reconciles a lost %s through a fresh read without repeating the write', async kind => {
  vi.mocked(api.getSessionCoordinationPolicy).mockResolvedValueOnce(policy(true));
  if (kind === 'write reply') vi.mocked(api.setSessionCoordinationPolicy).mockRejectedValueOnce(new Error('reply lost'));
  else vi.mocked(api.getSessionCoordinationPolicy).mockRejectedValueOnce(new Error('readback lost'));
  vi.mocked(api.getSessionCoordinationPolicy).mockResolvedValue(policy(false));
  render(<SessionCoordinationPolicy />);
  await waitFor(() => expect(checkbox()).toBeEnabled()); fireEvent.click(checkbox());
  expect(await screen.findByRole('alert')).toHaveTextContent('could not be confirmed');
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
  fireEvent.click(reload());
  await waitFor(() => expect(checkbox()).toBeEnabled()); expect(checkbox()).not.toBeChecked();
  expect(api.setSessionCoordinationPolicy).toHaveBeenCalledTimes(1);
});

it('shows a competing authoritative setting without claiming the requested value was saved', async () => {
  render(<SessionCoordinationPolicy />);
  await waitFor(() => expect(checkbox()).toBeEnabled()); fireEvent.click(checkbox());
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('differs from the requested change'));
  expect(checkbox()).toBeChecked();
});

it('ignores an aborted earlier mount read that returns after the current read', async () => {
  let finish!: (value: ReturnType<typeof policy>) => void;
  vi.mocked(api.getSessionCoordinationPolicy).mockImplementationOnce(() => new Promise(resolve => { finish = resolve; })).mockResolvedValue(policy(false));
  render(<StrictMode><SessionCoordinationPolicy /></StrictMode>);
  await waitFor(() => expect(checkbox()).toBeEnabled()); expect(checkbox()).not.toBeChecked();
  expect(vi.mocked(api.getSessionCoordinationPolicy).mock.calls[0][0]?.aborted).toBe(true);
  await act(async () => { finish(policy(true)); });
  expect(checkbox()).not.toBeChecked();
});
