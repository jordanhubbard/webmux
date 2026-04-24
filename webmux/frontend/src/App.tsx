import { useState, useEffect, useCallback, useMemo } from 'react';
import { useAuth } from './hooks/useAuth';
import type { AuthState } from './hooks/useAuth';
import { LoginPage } from './components/LoginPage';
import { TopBar } from './components/TopBar';
import { Workspace } from './components/Workspace';
import { GraphicsWorkspace } from './components/GraphicsWorkspace';
import { RegisterDialog } from './components/RegisterDialog';
import { InputBroadcastProvider } from './contexts/InputBroadcastContext';
import { WorkspacePaneProvider, useWorkspacePane } from './contexts/WorkspacePaneContext';
import { api } from './utils/api';
import type { NamedTheme } from './types';
import { loadBundledThemes, loadGlobalTheme, saveGlobalTheme } from './utils/themes';

interface AuthenticatedAppProps {
  auth: AuthState;
  fontSize: number;
  onFontSizeChange: (size: number) => void;
  termCols: number;
  termRows: number;
  onTermSizeChange: (cols: number, rows: number) => void;
  onNewAccount: () => void;
  secureMode: boolean;
  currentUser: string | null;
  showRegister: boolean;
  onRegisterClose: () => void;
  onAccountCreated: (username: string) => void;
  themes: NamedTheme[];
  globalTheme: string | null;
  onGlobalThemeChange: (name: string | null) => void;
}

function AuthenticatedApp({
  auth,
  fontSize,
  onFontSizeChange,
  termCols,
  termRows,
  onTermSizeChange,
  onNewAccount,
  secureMode,
  currentUser,
  showRegister,
  onRegisterClose,
  onAccountCreated,
  themes,
  globalTheme,
  onGlobalThemeChange,
}: AuthenticatedAppProps) {
  const { activePane } = useWorkspacePane();

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <TopBar
        auth={auth}
        fontSize={fontSize}
        onFontSizeChange={onFontSizeChange}
        termCols={termCols}
        termRows={termRows}
        onTermSizeChange={onTermSizeChange}
        onNewAccount={onNewAccount}
        secureMode={secureMode}
        currentUser={currentUser}
        themes={themes}
        globalTheme={globalTheme}
        onGlobalThemeChange={onGlobalThemeChange}
      />

      <div style={{ flex: 1, overflow: 'hidden', position: 'relative' }}>
        <div style={{ display: activePane === 'terminals' ? 'flex' : 'none', height: '100%', flexDirection: 'column' }}>
          <Workspace fontSize={fontSize} termCols={termCols} termRows={termRows} themes={themes} globalTheme={globalTheme} />
        </div>
        <div style={{ display: activePane === 'desktops' ? 'flex' : 'none', height: '100%', flexDirection: 'column' }}>
          <GraphicsWorkspace />
        </div>
      </div>

      {showRegister && (
        <RegisterDialog
          onClose={onRegisterClose}
          onCreated={onAccountCreated}
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

function saveTermSettings(fs: number, cols: number, rows: number) {
  api.updateConfig({ app: { default_term: { font_size: fs, cols, rows } } }).catch(() => {});
}

export default function App() {
  const auth = useAuth();
  const [fontSize, setFontSize] = useState(14);
  const [termCols, setTermCols] = useState(80);
  const [termRows, setTermRows] = useState(24);
  const [showRegister, setShowRegister] = useState(false);
  const [secureMode, setSecureMode] = useState(true);
  const [themes, setThemes] = useState<NamedTheme[]>([]);
  const [globalTheme, setGlobalTheme] = useState<string | null>(() => loadGlobalTheme());

  const currentUser = useMemo(() => auth.isAuthenticated ? parseTokenUser() : null, [auth.isAuthenticated]);

  useEffect(() => {
    api.getConfig().then(config => {
      setSecureMode(config.app.secure_mode);
      setFontSize(config.app.default_term.font_size);
      setTermCols(config.app.default_term.cols);
      setTermRows(config.app.default_term.rows);
    }).catch(() => {});
    loadBundledThemes().then(setThemes).catch(() => {});
  }, []);

  const handleGlobalThemeChange = useCallback((name: string | null) => {
    setGlobalTheme(name);
    saveGlobalTheme(name);
  }, []);

  const handleFontSizeChange = useCallback((size: number) => {
    setFontSize(size);
    saveTermSettings(size, termCols, termRows);
  }, [termCols, termRows]);

  const handleTermSizeChange = useCallback((cols: number, rows: number) => {
    setTermCols(cols);
    setTermRows(rows);
    saveTermSettings(fontSize, cols, rows);
  }, [fontSize]);

  const handleAccountCreated = useCallback((username: string) => {
    setShowRegister(false);
    alert(`Account "${username}" created. You can sign out and sign in as "${username}" to use it.`);
  }, []);

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
      <WorkspacePaneProvider>
        <AuthenticatedApp
          auth={auth}
          fontSize={fontSize}
          onFontSizeChange={handleFontSizeChange}
          termCols={termCols}
          termRows={termRows}
          onTermSizeChange={handleTermSizeChange}
          onNewAccount={() => setShowRegister(true)}
          secureMode={secureMode}
          currentUser={currentUser}
          showRegister={showRegister}
          onRegisterClose={() => setShowRegister(false)}
          onAccountCreated={handleAccountCreated}
          themes={themes}
          globalTheme={globalTheme}
          onGlobalThemeChange={handleGlobalThemeChange}
        />
      </WorkspacePaneProvider>
    </InputBroadcastProvider>
  );
}
