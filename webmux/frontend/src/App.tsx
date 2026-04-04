import { useState, useEffect, useCallback, useMemo } from 'react';
import { useAuth } from './hooks/useAuth';
import { LoginPage } from './components/LoginPage';
import { TopBar } from './components/TopBar';
import { Workspace } from './components/Workspace';
import { RegisterDialog } from './components/RegisterDialog';
import { InputBroadcastProvider } from './contexts/InputBroadcastContext';
import { api } from './utils/api';

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

  const currentUser = useMemo(() => auth.isAuthenticated ? parseTokenUser() : null, [auth.isAuthenticated]);

  useEffect(() => {
    api.getConfig().then(config => {
      setSecureMode(config.app.secure_mode);
      setFontSize(config.app.default_term.font_size);
      setTermCols(config.app.default_term.cols);
      setTermRows(config.app.default_term.rows);
    }).catch(() => {});
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
      <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        <TopBar
          auth={auth}
          fontSize={fontSize}
          onFontSizeChange={handleFontSizeChange}
          termCols={termCols}
          termRows={termRows}
          onTermSizeChange={handleTermSizeChange}
          onNewAccount={() => setShowRegister(true)}
          secureMode={secureMode}
          currentUser={currentUser}
        />

        <Workspace fontSize={fontSize} termCols={termCols} termRows={termRows} />

        {showRegister && (
          <RegisterDialog
            onClose={() => setShowRegister(false)}
            onCreated={handleAccountCreated}
          />
        )}
      </div>
    </InputBroadcastProvider>
  );
}
