import { Dialog } from './Dialog';
import { FormEvent, useEffect, useState } from 'react';
import type { AppConfig, WorkspaceName } from '../types';
import { api } from '../utils/api';
import { DEFAULT_TERMINAL_FONT_FAMILY } from '../utils/terminalFont';

interface WorkspaceOption {
  value: WorkspaceName;
  label: string;
}

interface SettingsDialogProps {
  workspaceOptions: WorkspaceOption[];
  onClose: () => void;
  onSaved: (config: AppConfig) => void;
}

interface SettingsForm {
  name: string;
  cols: string;
  rows: string;
  fontSize: string;
  fontFamily: string;
  maxCols: string;
  maxRows: string;
  sessionLogging: boolean;
  preferMosh: boolean;
  sshFallback: boolean;
  moshServerPath: string;
  defaultPane: WorkspaceName;
  fontFaces: Array<{
    family: string;
    source: string;
    weight: string;
    style: string;
    display: string;
  }>;
  hostSwitcherEnabled: boolean;
  hostSwitcherSuffixes: string;
  hostSwitcherHosts: Array<{
    id: string;
    label: string;
    hostname: string;
  }>;
}

function formFromConfig(config: AppConfig): SettingsForm {
  return {
    name: config.app.name,
    cols: String(config.app.default_term.cols),
    rows: String(config.app.default_term.rows),
    fontSize: String(config.app.default_term.font_size),
    fontFamily: config.app.default_term.font_family ?? DEFAULT_TERMINAL_FONT_FAMILY,
    maxCols: config.app.terminal_grid?.max_cols ? String(config.app.terminal_grid.max_cols) : '',
    maxRows: config.app.terminal_grid?.max_rows ? String(config.app.terminal_grid.max_rows) : '',
    sessionLogging: config.app.session_logging?.enabled === true,
    preferMosh: config.app.transport?.prefer_mosh === true,
    sshFallback: config.app.transport?.ssh_fallback !== false,
    moshServerPath: config.app.transport?.mosh_server_path ?? '',
    defaultPane: config.app.ui?.default_pane ?? 'terminals',
    fontFaces: (config.app.font_faces ?? []).map(face => ({
      family: face.family,
      source: face.source,
      weight: face.weight === undefined ? '' : String(face.weight),
      style: face.style ?? '',
      display: face.display ?? '',
    })),
    hostSwitcherEnabled: config.app.ui?.host_switcher?.enabled === true,
    hostSwitcherSuffixes: (config.app.ui?.host_switcher?.suffixes ?? []).join(', '),
    hostSwitcherHosts: (config.app.ui?.host_switcher?.hosts ?? []).map(host => ({
      id: host.id,
      label: host.label ?? '',
      hostname: host.hostname,
    })),
  };
}

function boundedInteger(value: string, label: string, min: number, max: number): number {
  if (!/^\d+$/.test(value.trim())) throw new Error(`${label} must be a whole number`);
  const parsed = Number(value);
  if (parsed < min || parsed > max) throw new Error(`${label} must be between ${min} and ${max}`);
  return parsed;
}

function gridLimit(value: string, label: string): number | null {
  if (!value.trim()) return null;
  if (!/^\d+$/.test(value.trim()) || Number(value) < 1) {
    throw new Error(`${label} must be blank or a positive whole number`);
  }
  return Number(value);
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <fieldset style={styles.section}>
      <legend style={styles.legend}>{title}</legend>
      {children}
    </fieldset>
  );
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <label style={styles.field}>
      <span style={styles.label}>{label}</span>
      {children}
      {hint && <span style={styles.hint}>{hint}</span>}
    </label>
  );
}

export function SettingsDialog({ workspaceOptions, onClose, onSaved }: SettingsDialogProps) {
  const [form, setForm] = useState<SettingsForm | null>(null);
  const [originalForm, setOriginalForm] = useState<SettingsForm | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    api.getConfig()
      .then(config => {
        if (!cancelled) {
          const loaded = formFromConfig(config);
          setForm(loaded);
          setOriginalForm(loaded);
        }
      })
      .catch(err => {
        if (!cancelled) setError((err as Error).message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, []);

  const update = <K extends keyof SettingsForm>(key: K, value: SettingsForm[K]) => {
    setForm(current => current ? { ...current, [key]: value } : current);
    setNotice(null);
  };

  const updateFontFace = (index: number, key: keyof SettingsForm['fontFaces'][number], value: string) => {
    if (!form) return;
    update('fontFaces', form.fontFaces.map((face, faceIndex) => faceIndex === index ? { ...face, [key]: value } : face));
  };

  const updateHost = (index: number, key: keyof SettingsForm['hostSwitcherHosts'][number], value: string) => {
    if (!form) return;
    update('hostSwitcherHosts', form.hostSwitcherHosts.map((host, hostIndex) => hostIndex === index ? { ...host, [key]: value } : host));
  };

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    if (!form || !originalForm) return;
    setError(null);
    setNotice(null);

    try {
      const name = form.name.trim();
      if (!name) throw new Error('Application name is required');
      const cols = boundedInteger(form.cols, 'Columns', 40, 240);
      const rows = boundedInteger(form.rows, 'Rows', 10, 80);
      const fontSize = boundedInteger(form.fontSize, 'Font size', 8, 32);
      const maxCols = gridLimit(form.maxCols, 'Maximum grid columns');
      const maxRows = gridLimit(form.maxRows, 'Maximum grid rows');
      for (const face of form.fontFaces) {
        if (!face.family.trim() || !face.source.trim()) throw new Error('Every font face needs a family and source path');
      }
      for (const host of form.hostSwitcherHosts) {
        if (!host.id.trim() || !host.hostname.trim()) throw new Error('Every host switcher entry needs an ID and hostname');
      }

      const terminalChanged = ['cols', 'rows', 'fontSize', 'fontFamily'].some(key =>
        form[key as keyof SettingsForm] !== originalForm[key as keyof SettingsForm]);
      const gridChanged = form.maxCols !== originalForm.maxCols || form.maxRows !== originalForm.maxRows;
      const transportChanged = form.preferMosh !== originalForm.preferMosh
        || form.sshFallback !== originalForm.sshFallback
        || form.moshServerPath !== originalForm.moshServerPath;
      const hostSwitcherChanged = form.hostSwitcherEnabled !== originalForm.hostSwitcherEnabled
        || form.hostSwitcherSuffixes !== originalForm.hostSwitcherSuffixes
        || JSON.stringify(form.hostSwitcherHosts) !== JSON.stringify(originalForm.hostSwitcherHosts);
      const fontFacesChanged = JSON.stringify(form.fontFaces) !== JSON.stringify(originalForm.fontFaces);

      const appUpdate = {
        ...(form.name !== originalForm.name ? { name } : {}),
        ...(terminalChanged ? {
          default_term: {
            cols,
            rows,
            font_size: fontSize,
            font_family: form.fontFamily.trim() || DEFAULT_TERMINAL_FONT_FAMILY,
          },
        } : {}),
        ...(gridChanged ? { terminal_grid: { max_cols: maxCols, max_rows: maxRows } } : {}),
        ...(form.sessionLogging !== originalForm.sessionLogging
          ? { session_logging: { enabled: form.sessionLogging } }
          : {}),
        ...(transportChanged ? {
          transport: {
            prefer_mosh: form.preferMosh,
            ssh_fallback: form.sshFallback,
            mosh_server_path: form.moshServerPath.trim(),
          },
        } : {}),
        ...(fontFacesChanged ? {
          font_faces: form.fontFaces.map(face => ({
            family: face.family.trim(),
            source: face.source.trim(),
            ...(face.weight.trim() ? { weight: face.weight.trim() } : {}),
            ...(face.style ? { style: face.style } : {}),
            ...(face.display ? { display: face.display as 'auto' | 'block' | 'swap' | 'fallback' | 'optional' } : {}),
          })),
        } : {}),
        ...(form.defaultPane !== originalForm.defaultPane || hostSwitcherChanged ? {
          ui: {
            ...(form.defaultPane !== originalForm.defaultPane ? { default_pane: form.defaultPane } : {}),
            ...(hostSwitcherChanged ? {
              host_switcher: {
                enabled: form.hostSwitcherEnabled,
                suffixes: form.hostSwitcherSuffixes.split(/[\n,]/).map(value => value.trim()).filter(Boolean),
                hosts: form.hostSwitcherHosts.map(host => ({
                  id: host.id.trim(),
                  ...(host.label.trim() ? { label: host.label.trim() } : {}),
                  hostname: host.hostname.trim(),
                })),
              },
            } : {}),
          },
        } : {}),
      };

      if (Object.keys(appUpdate).length === 0) {
        setNotice('No changes to save');
        return;
      }

      setSaving(true);
      const saved = await api.updateConfig({
        app: appUpdate,
      });
      const updated = formFromConfig(saved);
      setForm(updated);
      setOriginalForm(updated);
      onSaved(saved);
      setNotice('Settings saved');
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const paneOptions = form && !workspaceOptions.some(option => option.value === form.defaultPane)
    ? [...workspaceOptions, { value: form.defaultPane, label: form.defaultPane }]
    : workspaceOptions;

  return (
    <Dialog title="Settings" subtitle="Customize your workspace and terminal defaults"
      onClose={onClose} dismissible={!saving} width={720}>
      {loading ? (
        <div style={styles.loading}>Loading settings…</div>
      ) : !form ? (
        <div style={styles.loading}>{error ?? 'Settings could not be loaded.'}</div>
      ) : (
        <form onSubmit={handleSubmit} style={styles.form}>
          <div style={styles.body}>
            <Section title="General">
              <Field label="Application name">
                <input style={styles.input} value={form.name} onChange={event => update('name', event.target.value)} />
              </Field>
              <Field label="Default workspace" hint="The workspace selected when a browser first opens WebMux.">
                <select style={styles.input} value={form.defaultPane} onChange={event => update('defaultPane', event.target.value)}>
                  {paneOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
                </select>
              </Field>
            </Section>

            <Section title="Terminal defaults">
              <div style={styles.gridThree}>
                <Field label="Columns"><input aria-label="Default columns" style={styles.input} inputMode="numeric" value={form.cols} onChange={event => update('cols', event.target.value)} /></Field>
                <Field label="Rows"><input aria-label="Default rows" style={styles.input} inputMode="numeric" value={form.rows} onChange={event => update('rows', event.target.value)} /></Field>
                <Field label="Font size"><input aria-label="Default font size" style={styles.input} inputMode="numeric" value={form.fontSize} onChange={event => update('fontSize', event.target.value)} /></Field>
              </div>
              <Field label="Font family" hint="CSS font-family list used by newly opened terminals.">
                <input style={styles.input} value={form.fontFamily} onChange={event => update('fontFamily', event.target.value)} />
              </Field>
            </Section>

            <Section title="Terminal workspace limits">
              <div style={styles.gridTwo}>
                <Field label="Maximum columns" hint="Blank means unlimited.">
                  <input style={styles.input} inputMode="numeric" value={form.maxCols} onChange={event => update('maxCols', event.target.value)} />
                </Field>
                <Field label="Maximum rows" hint="Blank means unlimited.">
                  <input style={styles.input} inputMode="numeric" value={form.maxRows} onChange={event => update('maxRows', event.target.value)} />
                </Field>
              </div>
            </Section>

            <Section title="Session transcripts">
              <label style={styles.checkRow}>
                <input type="checkbox" checked={form.sessionLogging} onChange={event => update('sessionLogging', event.target.checked)} />
                <span>
                  <strong>Log terminal sessions to disk</strong>
                  <span style={styles.hint}>Applies to new and reconnected terminals. Logs may contain commands, output, tokens, and other secrets.</span>
                </span>
              </label>
            </Section>

            <Section title="Transport">
              <label style={styles.checkRow}>
                <input type="checkbox" checked={form.preferMosh} onChange={event => update('preferMosh', event.target.checked)} />
                Prefer mosh for hosts that allow it
              </label>
              <label style={styles.checkRow}>
                <input type="checkbox" checked={form.sshFallback} onChange={event => update('sshFallback', event.target.checked)} />
                Fall back to SSH when mosh cannot connect
              </label>
              <Field label="Mosh server path" hint="Optional absolute path to mosh-server; leave blank to use PATH.">
                <input style={styles.input} value={form.moshServerPath} onChange={event => update('moshServerPath', event.target.value)} placeholder="/usr/local/bin/mosh-server" />
              </Field>
            </Section>

            <Section title="Hosted terminal fonts">
              <span style={styles.hint}>Font source paths are relative to the server's app.yaml file.</span>
              {form.fontFaces.map((face, index) => (
                <div key={index} style={styles.dynamicRow}>
                  <input aria-label={`Font ${index + 1} family`} style={styles.input} value={face.family} onChange={event => updateFontFace(index, 'family', event.target.value)} placeholder="Family" />
                  <input aria-label={`Font ${index + 1} source`} style={styles.input} value={face.source} onChange={event => updateFontFace(index, 'source', event.target.value)} placeholder="fonts/MyFont.woff2" />
                  <input aria-label={`Font ${index + 1} weight`} style={styles.input} value={face.weight} onChange={event => updateFontFace(index, 'weight', event.target.value)} placeholder="Weight" />
                  <select aria-label={`Font ${index + 1} style`} style={styles.input} value={face.style} onChange={event => updateFontFace(index, 'style', event.target.value)}>
                    <option value="">Default style</option><option value="normal">Normal</option><option value="italic">Italic</option><option value="oblique">Oblique</option>
                  </select>
                  <select aria-label={`Font ${index + 1} display`} style={styles.input} value={face.display} onChange={event => updateFontFace(index, 'display', event.target.value)}>
                    <option value="">Default display</option><option value="auto">Auto</option><option value="block">Block</option><option value="swap">Swap</option><option value="fallback">Fallback</option><option value="optional">Optional</option>
                  </select>
                  <button type="button" style={styles.removeButton} onClick={() => update('fontFaces', form.fontFaces.filter((_, faceIndex) => faceIndex !== index))} aria-label={`Remove font ${index + 1}`}>Remove</button>
                </div>
              ))}
              <button type="button" style={styles.addButton} onClick={() => update('fontFaces', [...form.fontFaces, { family: '', source: '', weight: '', style: '', display: '' }])}>Add font face</button>
            </Section>

            <Section title="Host switcher">
              <label style={styles.checkRow}>
                <input type="checkbox" checked={form.hostSwitcherEnabled} onChange={event => update('hostSwitcherEnabled', event.target.checked)} />
                Show configured WebMux hosts in the top bar
              </label>
              <Field label="Allowed hostname suffixes" hint="Comma-separated. Leave blank to show the switcher on every hostname.">
                <input style={styles.input} value={form.hostSwitcherSuffixes} onChange={event => update('hostSwitcherSuffixes', event.target.value)} placeholder="example.com, lab.example.net" />
              </Field>
              {form.hostSwitcherHosts.map((host, index) => (
                <div key={index} style={styles.hostRow}>
                  <input aria-label={`Host ${index + 1} ID`} style={styles.input} value={host.id} onChange={event => updateHost(index, 'id', event.target.value)} placeholder="ID" />
                  <input aria-label={`Host ${index + 1} label`} style={styles.input} value={host.label} onChange={event => updateHost(index, 'label', event.target.value)} placeholder="Label" />
                  <input aria-label={`Host ${index + 1} hostname`} style={styles.input} value={host.hostname} onChange={event => updateHost(index, 'hostname', event.target.value)} placeholder="webmux.example.com" />
                  <button type="button" style={styles.removeButton} onClick={() => update('hostSwitcherHosts', form.hostSwitcherHosts.filter((_, hostIndex) => hostIndex !== index))} aria-label={`Remove host ${index + 1}`}>Remove</button>
                </div>
              ))}
              <button type="button" style={styles.addButton} onClick={() => update('hostSwitcherHosts', [...form.hostSwitcherHosts, { id: '', label: '', hostname: '' }])}>Add host</button>
            </Section>

            <div style={styles.safetyNote}>
              Startup and security settings—including ports, bind addresses, authentication mode, and secrets—remain file-managed and require a restart when applicable.
            </div>
          </div>

          <div style={styles.footer}>
            <div aria-live="polite">
              {error && <span style={styles.error}>{error}</span>}
              {!error && notice && <span style={styles.notice}>{notice}</span>}
            </div>
            <div style={styles.actions}>
              <button type="button" style={styles.secondaryButton} onClick={onClose} disabled={saving}>Close</button>
              <button type="submit" style={styles.primaryButton} disabled={saving}>{saving ? 'Saving…' : 'Save settings'}</button>
            </div>
          </div>
        </form>
      )}
    </Dialog>
  );
}

const styles: Record<string, React.CSSProperties> = {
  loading: { minHeight: 180, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#999' },
  form: { minHeight: 0, display: 'flex', flexDirection: 'column' },
  body: { padding: 18, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 14 },
  section: {
    margin: 0, padding: '12px 14px 14px', display: 'flex', flexDirection: 'column', gap: 11,
    border: '1px solid #303052', borderRadius: 7, background: '#121224',
  },
  legend: { padding: '0 7px', color: '#a99cff', fontSize: 13, fontWeight: 700 },
  field: { display: 'flex', flexDirection: 'column', gap: 5, minWidth: 0 },
  label: { color: '#d4d4df', fontSize: 12, fontWeight: 600 },
  hint: { display: 'block', color: '#77778e', fontSize: 11, fontWeight: 400, lineHeight: 1.4, marginTop: 2 },
  input: {
    width: '100%', boxSizing: 'border-box', padding: '8px 9px', color: '#eee', background: '#0c0c19',
    border: '1px solid #38385d', borderRadius: 4, outline: 'none', fontSize: 13,
  },
  gridThree: { display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 10 },
  gridTwo: { display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 10 },
  checkRow: { display: 'flex', alignItems: 'flex-start', gap: 9, color: '#d4d4df', fontSize: 13, lineHeight: 1.4 },
  dynamicRow: { display: 'grid', gridTemplateColumns: '1.2fr 1.7fr .7fr 1fr 1fr auto', gap: 6, alignItems: 'center' },
  hostRow: { display: 'grid', gridTemplateColumns: '.8fr 1fr 1.7fr auto', gap: 6, alignItems: 'center' },
  addButton: { alignSelf: 'flex-start', padding: '6px 9px', border: '1px solid #3d3d67', borderRadius: 4, background: '#232344', color: '#cfcff0', cursor: 'pointer', fontSize: 12 },
  removeButton: { padding: '7px 8px', border: '1px solid #673d4c', borderRadius: 4, background: '#38202a', color: '#f0a5b8', cursor: 'pointer', fontSize: 11 },
  safetyNote: {
    padding: '10px 12px', color: '#b6a66d', background: '#29230f', border: '1px solid #55491b',
    borderRadius: 5, fontSize: 11, lineHeight: 1.5,
  },
  footer: {
    minHeight: 42, display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12,
    padding: '12px 18px', borderTop: '1px solid #303052', background: '#111120',
  },
  actions: { display: 'flex', gap: 8, marginLeft: 'auto' },
  secondaryButton: { padding: '8px 13px', border: '1px solid #444467', borderRadius: 4, background: '#24243d', color: '#ddd', cursor: 'pointer' },
  primaryButton: { padding: '8px 13px', border: 0, borderRadius: 4, background: '#7c6af7', color: '#fff', cursor: 'pointer', fontWeight: 700 },
  error: { color: '#ff7777', fontSize: 12 },
  notice: { color: '#63d887', fontSize: 12 },
};
