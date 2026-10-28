import { beforeEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { InputBroadcastProvider, useInputBroadcast } from '@frontend/contexts/InputBroadcastContext';
import { OpenRigWorkspace } from '@frontend/components/OpenRigWorkspace';
import type { Session } from '@frontend/types';

const mocks = vi.hoisted(() => ({ status: vi.fn(), attach: vi.fn(), remove: vi.fn(), stop: vi.fn(), agents: vi.fn() }));
vi.mock('@frontend/utils/api', () => ({ api: { getOpenRig: mocks.status, attachOpenRig: mocks.attach, deleteSession: mocks.remove, stopOpenRigProof: mocks.stop, getOpenRigAgents: mocks.agents } }));
vi.mock('@frontend/components/Terminal', () => ({ Terminal: ({ onStateChange }: { onStateChange: (state: string) => void }) => {
  const { broadcastMode } = useInputBroadcast();
  return <div>Dashboard broadcast: {String(broadcastMode)}<button onClick={() => onStateChange('disconnected')}>Simulate dashboard exit</button></div>;
} }));
const session = { id: 'dashboard-one', state: 'connected', companion: 'openrig' } as Session;
beforeEach(() => {
  vi.clearAllMocks();
  mocks.status.mockResolvedValue({ available: true, message: '', session: null });
  mocks.attach.mockImplementation(async () => { mocks.status.mockResolvedValue({ available: true, message: '', session }); return session; });
  mocks.remove.mockResolvedValue(undefined);
  mocks.agents.mockResolvedValue([]);
});

it('reports a restored disconnected dashboard and reattaches it explicitly', async () => {
  mocks.status.mockResolvedValue({ available: true, message: '', session: { ...session, state: 'disconnected' } });
  render(<OpenRigWorkspace fontSize={14} poppedOut />);
  expect(await screen.findByText('Dashboard disconnected')).toBeVisible();
  expect(screen.queryByText('Dashboard connected')).toBeNull();
  expect(screen.getByText(/For Agent Deck sessions/)).toBeVisible();
  expect(mocks.attach).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Reconnect dashboard' }));
  expect(await screen.findByText('Dashboard connected')).toBeVisible();
  expect(mocks.attach).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole('button', { name: 'Simulate dashboard exit' }));
  expect(await screen.findByText('Dashboard disconnected')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Reconnect dashboard' })).toBeEnabled();
});

it('does not launch on selection, explains missing prerequisites and reports attachment failures', async () => {
  mocks.status.mockResolvedValue({ available: false, message: 'Install OpenRig on this WebMux server.', session: null });
  const view = render(<OpenRigWorkspace fontSize={14} />);
  await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Install OpenRig'));
  expect(screen.getByRole('button', { name: 'Connect dashboard' })).toBeDisabled();
  expect(mocks.attach).not.toHaveBeenCalled();
  view.unmount();
  mocks.status.mockResolvedValue({ available: true, message: '', session: null });
  mocks.attach.mockRejectedValue(new Error('OpenRig is unavailable'));
  render(<OpenRigWorkspace fontSize={14} />);
  await waitFor(() => expect(screen.getByRole('button', { name: 'Connect dashboard' })).toBeEnabled());
  fireEvent.click(screen.getByRole('button', { name: 'Connect dashboard' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('OpenRig is unavailable');
});

it('keeps the dashboard outside global broadcast and retains it when the view closes', async () => {
  function EnableBroadcast() {
    const broadcast = useInputBroadcast();
    return <button onClick={() => broadcast.setBroadcastMode(true)}>Broadcast</button>;
  }
  const view = render(<InputBroadcastProvider><EnableBroadcast /><OpenRigWorkspace fontSize={14} /></InputBroadcastProvider>);
  fireEvent.click(screen.getByRole('button', { name: 'Broadcast' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Connect dashboard' })).toBeEnabled());
  fireEvent.click(screen.getByRole('button', { name: 'Connect dashboard' }));
  expect(await screen.findByText('Dashboard broadcast: false')).toBeVisible();
  view.unmount();
  expect(mocks.remove).not.toHaveBeenCalled();
  mocks.status.mockResolvedValue({ available: true, message: '', session });
  render(<OpenRigWorkspace fontSize={14} />);
  expect(await screen.findByTestId('openrig-terminal')).toHaveAttribute('data-session-id', session.id);
  expect(mocks.attach).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole('button', { name: 'Detach dashboard' }));
  await waitFor(() => expect(screen.queryByTestId('openrig-terminal')).toBeNull());
  expect(mocks.remove).toHaveBeenCalledWith(session.id);
});

it('shows an actionable diagnostics connection and rechecks availability without launching agents', async () => {
  mocks.status.mockResolvedValueOnce({ available: false, message: 'Coordinator starting', session: null });
  render(<OpenRigWorkspace fontSize={14} />);
  await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Coordinator starting'));
  expect(screen.getByRole('heading', { name: 'Open Rig diagnostics' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Connect dashboard' })).toBeDisabled();
  mocks.status.mockResolvedValueOnce({ available: true, message: '', session: null });
  fireEvent.click(screen.getByRole('button', { name: 'Refresh connection' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Connect dashboard' })).toBeEnabled());
  expect(mocks.attach).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Connect dashboard' }));
  expect(await screen.findByTestId('openrig-terminal')).toHaveAttribute('data-session-id', session.id);
});
