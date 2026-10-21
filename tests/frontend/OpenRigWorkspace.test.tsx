import { beforeEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { InputBroadcastProvider, useInputBroadcast } from '@frontend/contexts/InputBroadcastContext';
import { OpenRigWorkspace } from '@frontend/components/OpenRigWorkspace';
import type { Session } from '@frontend/types';

const mocks = vi.hoisted(() => ({ status: vi.fn(), attach: vi.fn(), remove: vi.fn(), stop: vi.fn(), agents: vi.fn() }));
vi.mock('@frontend/utils/api', () => ({ api: { getOpenRig: mocks.status, attachOpenRig: mocks.attach, deleteSession: mocks.remove, stopOpenRigProof: mocks.stop, getOpenRigAgents: mocks.agents } }));
vi.mock('@frontend/components/Terminal', () => ({ Terminal: () => {
  const { broadcastMode } = useInputBroadcast();
  return <div>Dashboard broadcast: {String(broadcastMode)}</div>;
} }));
const session = { id: 'dashboard-one', state: 'connected', companion: 'openrig' } as Session;
beforeEach(() => {
  vi.clearAllMocks();
  mocks.status.mockResolvedValue({ available: true, message: '', session: null });
  mocks.attach.mockImplementation(async () => { mocks.status.mockResolvedValue({ available: true, message: '', session }); return session; });
  mocks.remove.mockResolvedValue(undefined);
  mocks.agents.mockResolvedValue([]);
});

it('does not launch on selection, explains missing prerequisites and reports attachment failures', async () => {
  mocks.status.mockResolvedValue({ available: false, message: 'Install OpenRig on this WebMux server.', session: null });
  const view = render(<OpenRigWorkspace fontSize={14} />);
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Install OpenRig'));
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

it('opens the authenticated panel route and reports blocked pop-ups', async () => {
  const open = vi.spyOn(window, 'open').mockReturnValue(null);
  try {
    render(<OpenRigWorkspace fontSize={14} />);
    fireEvent.click(screen.getByRole('button', { name: 'Pop out' }));
    expect(open).toHaveBeenCalledWith(expect.stringContaining('panel=openrig'), 'webmux-openrig-dashboard', expect.any(String));
    expect(await screen.findByRole('alert')).toHaveTextContent('Allow pop-ups');
  } finally { open.mockRestore(); }
});

it('keeps real agent views separate, pops out the selected CLI, and stops only on explicit request', async () => {
  const proof = { phase: 'passed', run_id: 'fresh-run', nonce_a: 'challenge', nonce_b: 'response', digest: 'independent-hash', message: 'Verified response and ACK' };
  const alpha = { ...session, id: 'claude-one', openrig_view: 'alpha' };
  mocks.status.mockResolvedValue({ available: true, message: '', session, views: { dashboard: session, alpha }, proof });
  mocks.stop.mockResolvedValue(undefined);
  const open = vi.spyOn(window, 'open').mockReturnValue(null);
  try {
    render(<OpenRigWorkspace fontSize={14} />);
    expect(await screen.findByText('independent-hash')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Claude' }));
    expect(screen.getByTestId('openrig-terminal')).toHaveAttribute('data-session-id', alpha.id);
    fireEvent.click(screen.getByRole('button', { name: 'Pop out' }));
    expect(open).toHaveBeenCalledWith(expect.stringContaining('view=alpha'), 'webmux-openrig-alpha', expect.any(String));
    fireEvent.click(screen.getByRole('button', { name: 'Codex' }));
    expect(screen.queryByTestId('openrig-terminal')).toBeNull();
    expect(mocks.attach).not.toHaveBeenCalled();
    expect(mocks.stop).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Stop test agents' }));
    await waitFor(() => expect(mocks.stop).toHaveBeenCalledOnce());
  } finally { open.mockRestore(); }
});

it('starts a live test only by explicit action and does not retain the old PASS while starting', async () => {
  mocks.status.mockResolvedValue({ available: true, message: '', session: null, proof: { phase: 'passed', run_id: 'old', message: 'old pass', digest: 'old-hash' } });
  let finish!: (value: Session) => void;
  mocks.attach.mockImplementation(() => new Promise<Session>(resolve => { finish = resolve; }));
  render(<OpenRigWorkspace fontSize={14} />);
  expect(await screen.findByText('old-hash')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Live test console' }));
  expect(mocks.attach).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Run fresh challenge' }));
  expect(mocks.attach).toHaveBeenCalledWith('proof');
  expect(screen.queryByText('old-hash')).toBeNull();
  mocks.status.mockResolvedValue({ available: true, message: '', session: null, views: { proof: session }, proof: { phase: 'waiting', run_id: 'new', message: 'Waiting for real agents' } });
  finish(session);
  expect(await screen.findByText(/WAITING: Waiting for real agents/)).toBeVisible();
});

it('attaches daemon-listed agent terminals and preserves their selection in pop-outs', async () => {
  mocks.agents.mockResolvedValue([{ target: 'builder@my-project', runtime: 'codex', rig: 'my-project', status: 'running' }]);
  mocks.attach.mockResolvedValue({ ...session, id: 'builder-cli', openrig_view: 'agent:builder@my-project' });
  const open = vi.spyOn(window, 'open').mockReturnValue(null);
  try {
    render(<OpenRigWorkspace fontSize={14} />);
    fireEvent.click(await screen.findByRole('button', { name: 'builder@my-project' }));
    expect(mocks.attach).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Connect builder@my-project' }));
    await waitFor(() => expect(mocks.attach).toHaveBeenCalledWith('agent', 'builder@my-project'));
    fireEvent.click(screen.getByRole('button', { name: 'Pop out' }));
    const url = new URL(open.mock.calls[0][0] as string);
    expect(url.searchParams.get('view')).toBe('agent:builder@my-project');
  } finally { open.mockRestore(); }
});
