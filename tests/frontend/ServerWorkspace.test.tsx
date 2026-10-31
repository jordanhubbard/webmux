import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { ServerWorkspace } from '@frontend/components/ServerWorkspace';
import { InputBroadcastProvider } from '@frontend/contexts/InputBroadcastContext';
import type { Session } from '@frontend/types';

const mocks = vi.hoisted(() => ({ list: vi.fn(), create: vi.fn(), message: vi.fn(), restart: vi.fn() }));
vi.mock('@frontend/utils/api', () => ({ api: { getServerSessions: mocks.list, createServerSession: mocks.create, sendServerMessage: mocks.message, reconnectServerSession: mocks.restart } }));
vi.mock('@frontend/components/Terminal', () => ({ Terminal: ({ sessionId }: { sessionId: string }) => <div data-testid="remote-terminal">{sessionId}</div> }));
const servers = [{ id: 'local', name: 'This Mac' }, { id: 'remote', name: 'Other host' }];
const session = { id: 'same-id', title: 'Builder', project_label: 'WebMux', state: 'connected', hostname: 'localhost' } as Session;
const renderView = (selected = 'all') => render(<InputBroadcastProvider><ServerWorkspace servers={servers} selected={selected} fontSize={14} themes={[]} globalTheme={null} /></InputBroadcastProvider>);
beforeEach(() => { vi.clearAllMocks(); mocks.list.mockResolvedValue([session]); mocks.message.mockResolvedValue({ queued: true, processed: false }); });

describe('connected server workspace', () => {
  it('namespaces identical session IDs and hides only the chosen viewer', async () => {
    renderView();
    await waitFor(() => expect(screen.getAllByTestId('remote-terminal')).toHaveLength(2));
    expect(screen.getByText('@local/same-id')).toBeInTheDocument();
    expect(screen.getByText('@remote/same-id')).toBeInTheDocument();
    const remote = screen.getByTestId('server-terminal-remote-same-id');
    expect(remote).toHaveAccessibleName('Other host · WebMux / Builder');
    fireEvent.click(within(remote).getByRole('button', { name: 'Hide' }));
    expect(screen.queryByText('@remote/same-id')).not.toBeInTheDocument();
    expect(screen.getByText('@local/same-id')).toBeInTheDocument();
    expect(mocks.restart).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Show hidden viewers' }));
    expect(screen.getByText('@remote/same-id')).toBeInTheDocument();
    expect(mocks.create).not.toHaveBeenCalled();
  });
  it('loads only the selected host and sends to an explicit session with honest acknowledgement', async () => {
    renderView('remote');
    await waitFor(() => expect(mocks.list).toHaveBeenCalledWith('remote'));
    expect(mocks.list).not.toHaveBeenCalledWith('local');
    fireEvent.click(await screen.findByRole('button', { name: 'Message' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Message' }), { target: { value: 'Review this change' } });
    fireEvent.click(screen.getByRole('button', { name: 'Send to this session' }));
    await waitFor(() => expect(mocks.message).toHaveBeenCalledWith('remote', 'same-id', 'Review this change'));
    expect(await screen.findByRole('status')).toHaveTextContent('Agent processing is not confirmed');
    expect(mocks.restart).not.toHaveBeenCalled();
  });
  it('shows unavailable servers independently while other servers remain usable', async () => {
    mocks.list.mockImplementation((id: string) => id === 'remote' ? Promise.reject(new Error('offline')) : Promise.resolve([session]));
    renderView();
    expect(await screen.findByRole('alert')).toHaveTextContent('Unavailable');
    expect(screen.getByText('@local/same-id')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'New terminal on Other host' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'New terminal on This Mac' })).toBeEnabled();
  });
});
