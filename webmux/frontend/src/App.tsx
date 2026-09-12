import { useState, useEffect, useCallback, useMemo } from 'react';
import { useAuth } from './hooks/useAuth';
import type { AuthState } from './hooks/useAuth';
import { LoginPage } from './components/LoginPage';
import { TopBar } from './components/TopBar';
import { Workspace } from './components/Workspace';
import { GraphicsWorkspace } from './components/GraphicsWorkspace';
import { AgentWorkspace } from './components/AgentWorkspace';
import { UsersDialog } from './components/UsersDialog';
import { SettingsDialog } from './components/SettingsDialog';
import { Dialog } from './components/Dialog';
import { InputBroadcastProvider } from './contexts/InputBroadcastContext';
import { WorkspacePaneProvider, useWorkspacePane, type WorkspacePane } from './contexts/WorkspacePaneContext';
import { api } from './utils/api';
import type { AgentDefinition, AgentsConfig, AppConfig, AppFontFaceConfig, HostSwitcherConfig, NamedTheme } from './types';
import { loadBundledThemes, loadGlobalTheme, saveGlobalTheme } from './utils/themes';
import {
  DEFAULT_TERMINAL_FONT_FAMILY,
  TERMINAL_FONT_CSS_VARIABLE,
  installConfiguredFontFaces,
  normalizeTerminalFontFamily,
} from './utils/terminalFont';

interface AuthenticatedAppProps {
  auth: AuthState;
  appName: string;
  fontSize: number;
  fontFamily: string;
  onFontSizeChange: (size: number) => void;
  termCols: number;
  termRows: number;
  onTermSizeChange: (cols: number, rows: number) => void;
  terminalGridLimit: {
    maxCols: number | null;
    maxRows: number | null;
  };
  onManageUsers: () => void;
  onManageSettings: () => void;
  canManageSettings: boolean;
  secureMode: boolean;
  currentUser: string | null;
  showUsers: boolean;
  onUsersClose: () => void;
  themes: NamedTheme[];
  globalTheme: string | null;
  onGlobalThemeChange: (name: string | null) => void;
  globalAutoScroll: boolean;
  onGlobalAutoScrollChange: (on: boolean) => void;
  onGlobalAutoScrollSync: (on: boolean) => void;
  globalAutoScrollVersion: number;
  globalLock: boolean;
  onGlobalLockChange: (on: boolean) => void;
  onGlobalLockSync: (on: boolean) => void;
  globalLockVersion: number;
  agentConfig: AgentsConfig;
  hostSwitcher: HostSwitcherConfig;
}

const DEFAULT_AGENT_CONFIG: AgentsConfig = {
  enabled: false,
  combined_pane: true,
  disable_in_multi_user_mode: true,
  definitions: [],
};

const DEFAULT_HOST_SWITCHER: HostSwitcherConfig = {
  enabled: false,
  suffixes: [],
  hosts: [],
};

function enabledAgentDefinitions(config: AgentsConfig): AgentDefinition[] {
  return config.enabled ? config.definitions.filter(definition => definition.enabled) : [];
}

function AuthenticatedApp({
  auth,
  appName,
  fontSize,
  fontFamily,
  onFontSizeChange,
  termCols,
  termRows,
  onTermSizeChange,
  terminalGridLimit,
  onManageUsers,
  onManageSettings,
  canManageSettings,
  secureMode,
  currentUser,
  showUsers,
  onUsersClose,
  themes,
  globalTheme,
  onGlobalThemeChange,
  globalAutoScroll,
  onGlobalAutoScrollChange,
  onGlobalAutoScrollSync,
  globalAutoScrollVersion,
  globalLock,
  onGlobalLockChange,
  onGlobalLockSync,
  globalLockVersion,
  agentConfig,
  hostSwitcher,
}: AuthenticatedAppProps) {
  const { activePane, setActivePane } = useWorkspacePane();
  const agentDefinitions = useMemo(() => enabledAgentDefinitions(agentConfig), [agentConfig]);
  const combinedAgentPane = agentConfig.combined_pane !== false;
  const agentWorkspacePanes = useMemo(
    () => new Set<WorkspacePane>(combinedAgentPane ? ['agents'] : agentDefinitions.map(definition => definition.workspace)),
    [agentDefinitions, combinedAgentPane],
  );
  const [mountedAgentPanes, setMountedAgentPanes] = useState<Set<WorkspacePane>>(() => new Set());

  useEffect(() => {
    if (!agentWorkspacePanes.has(activePane)) return;
    setMountedAgentPanes(prev => {
      if (prev.has(activePane)) return prev;
      const next = new Set(prev);
      next.add(activePane);
      return next;
    });
  }, [activePane, agentWorkspacePanes]);

  const shouldMountAgentPane = useCallback(
    (pane: WorkspacePane) => activePane === pane || mountedAgentPanes.has(pane),
    [activePane, mountedAgentPanes],
  );

  const handleAgentAccessDenied = useCallback(() => {
    setActivePane('terminals');
  }, [setActivePane]);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <TopBar
        auth={auth}
        appName={appName}
        fontSize={fontSize}
        onFontSizeChange={onFontSizeChange}
        termCols={termCols}
        termRows={termRows}
        onTermSizeChange={onTermSizeChange}
        onManageUsers={onManageUsers}
        onManageSettings={onManageSettings}
        canManageSettings={canManageSettings}
        secureMode={secureMode}
        currentUser={currentUser}
        themes={themes}
        globalTheme={globalTheme}
        onGlobalThemeChange={onGlobalThemeChange}
        globalAutoScroll={globalAutoScroll}
        onGlobalAutoScrollChange={onGlobalAutoScrollChange}
        globalLock={globalLock}
        onGlobalLockChange={onGlobalLockChange}
        agentDefinitions={agentDefinitions}
        combinedAgentPane={combinedAgentPane}
        hostSwitcher={hostSwitcher}
      />

      <div style={{ flex: 1, overflow: 'hidden', position: 'relative' }}>
        <div style={{ display: activePane === 'terminals' ? 'flex' : 'none', height: '100%', flexDirection: 'column' }}>
          <Workspace
            fontSize={fontSize}
            fontFamily={fontFamily}
            termCols={termCols}
            termRows={termRows}
            terminalGridLimit={terminalGridLimit}
            themes={themes}
            globalTheme={globalTheme}
            globalAutoScroll={globalAutoScroll}
            globalAutoScrollVersion={globalAutoScrollVersion}
            onGlobalAutoScrollChange={onGlobalAutoScrollSync}
            globalLock={globalLock}
            globalLockVersion={globalLockVersion}
            onGlobalLockChange={onGlobalLockSync}
          />
        </div>
        <div style={{ display: activePane === 'desktops' ? 'flex' : 'none', height: '100%', flexDirection: 'column' }}>
          <GraphicsWorkspace />
        </div>
        {agentDefinitions.length > 0 && combinedAgentPane && shouldMountAgentPane('agents') && (
          <div style={{ display: activePane === 'agents' ? 'flex' : 'none', height: '100%', flexDirection: 'column' }}>
            <AgentWorkspace
              agentDefinitions={agentDefinitions}
              fontSize={fontSize}
              fontFamily={fontFamily}
              termCols={termCols}
              termRows={termRows}
              themes={themes}
              globalTheme={globalTheme}
              onAgentAccessDenied={handleAgentAccessDenied}
            />
          </div>
        )}
        {agentDefinitions.length > 0 && !combinedAgentPane && agentDefinitions.map(definition => (
          shouldMountAgentPane(definition.workspace) && (
            <div key={definition.id} style={{ display: activePane === definition.workspace ? 'flex' : 'none', height: '100%', flexDirection: 'column' }}>
              <AgentWorkspace
                agent={definition}
                agentDefinitions={agentDefinitions}
                fontSize={fontSize}
                fontFamily={fontFamily}
                termCols={termCols}
                termRows={termRows}
                themes={themes}
                globalTheme={globalTheme}
                onAgentAccessDenied={handleAgentAccessDenied}
              />
            </div>
          )
        ))}
      </div>

      {showUsers && (
        <UsersDialog
          currentUser={currentUser}
          onClose={onUsersClose}
        />
      )}
    </div>
  );
}

function parseTokenUser(): string | null {
  const token = localStorage.getItem('webmux_token');
  if (!token) return null;
  try {
    const payload = JSON.parse(atob(token.split('.')[1]));
    return payload.sub || null;
  } catch {
    return null;
  }
}

function saveTermSettings(fs: number, cols: number, rows: number, fontFamily: string) {
  api.updateConfig({ app: { default_term: { font_size: fs, cols, rows, font_family: fontFamily } } }).catch(() => {});
}

export default function App() {
  const auth = useAuth();
  const [appName, setAppName] = useState('WebMux');
  const [fontSize, setFontSize] = useState(14);
  const [fontFamily, setFontFamily] = useState(DEFAULT_TERMINAL_FONT_FAMILY);
  const [termCols, setTermCols] = useState(80);
  const [termRows, setTermRows] = useState(24);
  const [terminalGridLimit, setTerminalGridLimit] = useState<{ maxCols: number | null; maxRows: number | null }>({
    maxCols: null,
    maxRows: null,
  });
  const [showUsers, setShowUsers] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [secureMode, setSecureMode] = useState(true);
  const [themes, setThemes] = useState<NamedTheme[]>([]);
  const [globalTheme, setGlobalTheme] = useState<string | null>(() => loadGlobalTheme());
  const [globalAutoScroll, setGlobalAutoScroll] = useState(true);
  const [globalAutoScrollVersion, setGlobalAutoScrollVersion] = useState(0);
  const [globalLock, setGlobalLock] = useState(false);
  const [globalLockVersion, setGlobalLockVersion] = useState(0);
  const [defaultPane, setDefaultPane] = useState<WorkspacePane>('terminals');
  const [agentConfig, setAgentConfig] = useState<AgentsConfig>(DEFAULT_AGENT_CONFIG);
  const [hostSwitcher, setHostSwitcher] = useState<HostSwitcherConfig>(DEFAULT_HOST_SWITCHER);
  const [fontFaces, setFontFaces] = useState<AppFontFaceConfig[]>([]);

  const currentUser = useMemo(
    () => (auth.isAuthenticated ? (auth.username ?? parseTokenUser()) : null),
    [auth.isAuthenticated, auth.username],
  );
  const canManageSettings = auth.authStatus?.mode === 'none' || auth.isAdmin;

  const applyConfig = useCallback((config: AppConfig) => {
    setAppName(config.app.name);
    setSecureMode(config.app.secure_mode);
    setFontSize(config.app.default_term.font_size);
    setFontFamily(normalizeTerminalFontFamily(config.app.default_term.font_family));
    setTermCols(config.app.default_term.cols);
    setTermRows(config.app.default_term.rows);
    setTerminalGridLimit({
      maxCols: config.app.terminal_grid?.max_cols ?? null,
      maxRows: config.app.terminal_grid?.max_rows ?? null,
    });
    setDefaultPane(config.app.ui?.default_pane ?? 'terminals');
    setAgentConfig(config.app.agents ?? DEFAULT_AGENT_CONFIG);
    setHostSwitcher(config.app.ui?.host_switcher ?? DEFAULT_HOST_SWITCHER);
    setFontFaces(config.app.font_faces ?? []);
  }, []);

  useEffect(() => {
    if (auth.isLoading || !auth.isAuthenticated) return;

    let cancelled = false;
    api.getConfig().then(config => {
      if (cancelled) return;
      applyConfig(config);
    }).catch(() => {});
    return () => { cancelled = true; };
  }, [applyConfig, auth.isAuthenticated, auth.isLoading]);

  useEffect(() => {
    loadBundledThemes().then(setThemes).catch(() => {});
  }, []);

  useEffect(() => {
    document.documentElement.style.setProperty(TERMINAL_FONT_CSS_VARIABLE, fontFamily);
  }, [fontFamily]);

  useEffect(() => {
    installConfiguredFontFaces(fontFaces).catch(() => {});
  }, [fontFaces]);

  const handleGlobalThemeChange = useCallback((name: string | null) => {
    setGlobalTheme(name);
    saveGlobalTheme(name);
  }, []);

  const handleFontSizeChange = useCallback((size: number) => {
    setFontSize(size);
    if (canManageSettings) saveTermSettings(size, termCols, termRows, fontFamily);
  }, [canManageSettings, fontFamily, termCols, termRows]);

  const handleTermSizeChange = useCallback((cols: number, rows: number) => {
    setTermCols(cols);
    setTermRows(rows);
    if (canManageSettings) saveTermSettings(fontSize, cols, rows, fontFamily);
  }, [canManageSettings, fontFamily, fontSize]);

  const availablePanes = useMemo<WorkspacePane[]>(() => {
    const agentDefinitions = enabledAgentDefinitions(agentConfig);
    const panes: WorkspacePane[] = ['terminals', 'desktops'];
    if (agentDefinitions.length > 0) {
      if (agentConfig.combined_pane !== false) {
        panes.push('agents');
      } else {
        panes.push(...agentDefinitions.map(definition => definition.workspace));
      }
    }
    return panes;
  }, [agentConfig]);
  const workspaceOptions = useMemo(() => availablePanes.map(pane => ({
    value: pane,
    label: pane === 'terminals' ? 'Terminals' : pane === 'desktops' ? 'Desktops' : pane === 'agents'
      ? 'Agents'
      : agentConfig.definitions.find(definition => definition.workspace === pane)?.plural_label ?? pane,
  })), [agentConfig.definitions, availablePanes]);

  if (auth.isLoading) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#888' }}>
        Loading...
      </div>
    );
  }

  if (!auth.isAuthenticated) {
    return <LoginPage auth={auth} />;
  }

  return (
    <InputBroadcastProvider>
      <WorkspacePaneProvider defaultPane={defaultPane} availablePanes={availablePanes}>
        <AuthenticatedApp
          auth={auth}
          appName={appName}
          fontSize={fontSize}
          fontFamily={fontFamily}
          onFontSizeChange={handleFontSizeChange}
          termCols={termCols}
          termRows={termRows}
          onTermSizeChange={handleTermSizeChange}
          terminalGridLimit={terminalGridLimit}
          onManageUsers={() => setShowUsers(true)}
          onManageSettings={() => setShowSettings(true)}
          canManageSettings={canManageSettings}
          secureMode={secureMode}
          currentUser={currentUser}
          showUsers={showUsers}
          onUsersClose={() => setShowUsers(false)}
          themes={themes}
          globalTheme={globalTheme}
          onGlobalThemeChange={handleGlobalThemeChange}
          globalAutoScroll={globalAutoScroll}
          onGlobalAutoScrollChange={(on: boolean) => {
            setGlobalAutoScroll(on);
            setGlobalAutoScrollVersion(v => v + 1);
          }}
          onGlobalAutoScrollSync={setGlobalAutoScroll}
          globalAutoScrollVersion={globalAutoScrollVersion}
          globalLock={globalLock}
          onGlobalLockChange={(on: boolean) => {
            setGlobalLock(on);
            setGlobalLockVersion(v => v + 1);
          }}
          onGlobalLockSync={setGlobalLock}
          globalLockVersion={globalLockVersion}
          agentConfig={agentConfig}
          hostSwitcher={hostSwitcher}
        />
        {showSettings && canManageSettings && (
          <SettingsDialog
            workspaceOptions={workspaceOptions}
            onClose={() => setShowSettings(false)}
            onSaved={applyConfig}
          />
        )}
        {auth.sessionExpired && (
          <Dialog title="Session timed out" dismissible={false}>
            <div style={timeoutStyles.body}>
              <p style={timeoutStyles.message}>Your WebMux login session has expired. Sign in again to reconnect to your running sessions.</p>
              <button style={timeoutStyles.button} onClick={auth.logout}>Return to sign in</button>
            </div>
          </Dialog>
        )}
      </WorkspacePaneProvider>
    </InputBroadcastProvider>
  );
}

const timeoutStyles: Record<string, React.CSSProperties> = {
  body: { padding: 24 },
  message: { margin: '0 0 22px', color: '#bbb', lineHeight: 1.5 },
  button: {
    width: '100%', border: 0, borderRadius: 5, background: '#7c6af7', color: '#fff', cursor: 'pointer',
    fontSize: 14, fontWeight: 700, padding: '10px 16px',
  },
};
