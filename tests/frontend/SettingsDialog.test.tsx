import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { SettingsDialog } from '@frontend/components/SettingsDialog';
import { api } from '@frontend/utils/api';

vi.mock('@frontend/utils/api', () => ({
  api: {
    getConfig: vi.fn(),
    updateConfig: vi.fn(),
  },
}));

const config = {
  app: {
    name: 'WebMux',
    http_port: 8080,
    https_port: 8443,
    secure_mode: true,
    trusted_http_allowed: false,
    default_term: {
      cols: 80,
      rows: 24,
      font_size: 14,
      font_family: 'ui-monospace, monospace',
    },
    terminal_grid: { max_cols: null, max_rows: 4 },
    session_logging: { enabled: false },
    coordination: { default_provider: 'agent-deck' as const },
    transport: { prefer_mosh: false, ssh_fallback: true, mosh_server_path: '' },
    ui: { default_pane: 'terminals' },
  },
};

const workspaceOptions = [
  { value: 'terminals', label: 'Terminals' },
  { value: 'desktops', label: 'Desktops' },
];

describe('SettingsDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (api.getConfig as ReturnType<typeof vi.fn>).mockResolvedValue(config);
    (api.updateConfig as ReturnType<typeof vi.fn>).mockImplementation(async update => ({
      app: {
        ...config.app,
        ...update.app,
        default_term: { ...config.app.default_term, ...update.app.default_term },
        terminal_grid: { ...config.app.terminal_grid, ...update.app.terminal_grid },
        session_logging: { ...config.app.session_logging, ...update.app.session_logging },
        coordination: { ...config.app.coordination, ...update.app.coordination },
        transport: { ...config.app.transport, ...update.app.transport },
        ui: { ...config.app.ui, ...update.app.ui },
      },
    }));
  });

  it('loads persisted settings and enables transcript logging', async () => {
    const onSaved = vi.fn();
    render(<SettingsDialog workspaceOptions={workspaceOptions} onClose={vi.fn()} onSaved={onSaved} />);

    expect(await screen.findByDisplayValue('WebMux')).toBeDefined();
    const logging = screen.getByRole('checkbox', { name: /Log terminal sessions to disk/ });
    expect((logging as HTMLInputElement).checked).toBe(false);
    fireEvent.click(logging);
    fireEvent.change(screen.getByLabelText('Application name'), { target: { value: 'Operations Mux' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }));

    await waitFor(() => {
      expect(api.updateConfig).toHaveBeenCalledWith({
        app: {
          name: 'Operations Mux',
          session_logging: { enabled: true },
        },
      });
      expect(onSaved).toHaveBeenCalledTimes(1);
    });
    expect(await screen.findByText('Settings saved')).toBeDefined();
  });

  it('manages hosted fonts and host switcher entries', async () => {
    render(<SettingsDialog workspaceOptions={workspaceOptions} onClose={vi.fn()} onSaved={vi.fn()} />);
    await screen.findByDisplayValue('WebMux');

    fireEvent.click(screen.getByRole('button', { name: 'Add font face' }));
    fireEvent.change(screen.getByLabelText('Font 1 family'), { target: { value: 'Fixture Mono' } });
    fireEvent.change(screen.getByLabelText('Font 1 source'), { target: { value: 'fonts/fixture.woff2' } });
    fireEvent.change(screen.getByLabelText('Font 1 weight'), { target: { value: '400' } });
    fireEvent.click(screen.getByRole('checkbox', { name: /Show configured WebMux hosts/ }));
    fireEvent.change(screen.getByLabelText(/Allowed hostname suffixes/), { target: { value: 'example.com' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add host' }));
    fireEvent.change(screen.getByLabelText('Host 1 ID'), { target: { value: 'lab' } });
    fireEvent.change(screen.getByLabelText('Host 1 label'), { target: { value: 'Lab' } });
    fireEvent.change(screen.getByLabelText('Host 1 hostname'), { target: { value: 'webmux.example.com' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }));

    await waitFor(() => expect(api.updateConfig).toHaveBeenCalledWith({
      app: {
        font_faces: [{ family: 'Fixture Mono', source: 'fonts/fixture.woff2', weight: '400' }],
        ui: {
          host_switcher: {
            enabled: true,
            suffixes: ['example.com'],
            hosts: [{ id: 'lab', label: 'Lab', hostname: 'webmux.example.com' }],
          },
        },
      },
    }));
  });

  it('validates terminal dimensions before saving', async () => {
    render(<SettingsDialog workspaceOptions={workspaceOptions} onClose={vi.fn()} onSaved={vi.fn()} />);
    fireEvent.change(await screen.findByLabelText('Default columns'), { target: { value: '12' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }));

    expect(await screen.findByText('Columns must be between 40 and 240')).toBeDefined();
    expect(api.updateConfig).not.toHaveBeenCalled();
  });

  it('defaults missing coordination settings to WebMux', async () => {
    (api.getConfig as ReturnType<typeof vi.fn>).mockResolvedValue({ ...config, app: { ...config.app, coordination: undefined } });
    render(<SettingsDialog workspaceOptions={workspaceOptions} onClose={vi.fn()} onSaved={vi.fn()} />);
    expect(await screen.findByLabelText('Default coordination experience')).toHaveValue('webmux');
  });

  it('changes the default coordination experience', async () => {
    render(<SettingsDialog workspaceOptions={workspaceOptions} onClose={vi.fn()} onSaved={vi.fn()} />);
    fireEvent.change(await screen.findByLabelText('Default coordination experience'), { target: { value: 'rig' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }));

    await waitFor(() => expect(api.updateConfig).toHaveBeenCalledWith({
      app: { coordination: { default_provider: 'rig' } },
    }));
  });

  it('reports save errors without closing the dialog', async () => {
    (api.updateConfig as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('Admin privileges required'));
    render(<SettingsDialog workspaceOptions={workspaceOptions} onClose={vi.fn()} onSaved={vi.fn()} />);
    await screen.findByDisplayValue('WebMux');
    fireEvent.click(screen.getByRole('checkbox', { name: /Log terminal sessions to disk/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Save settings' }));

    expect(await screen.findByText('Admin privileges required')).toBeDefined();
    expect(screen.getByRole('dialog', { name: 'Settings' })).toBeDefined();
  });
});
