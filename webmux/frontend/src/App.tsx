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

export default function App() {
  const auth = useAuth();
  const [fontSize, setFontSize] = useState(14);
  const [showRegister, setShowRegister] = useState(false);
  const [secureMode, setSecureMode] = useState(true);

  const currentUser = useMemo(() => auth.isAuthenticated ? parseTokenUser() : null, [auth.isAuthenticated]);

  useEffect(() => {
    api.getConfig().then(config => {
      setSecureMode(config.app.secure_mode);
      setFontSize(config.app.default_term.font_size);
    }).catch(() => {});
  }, []);

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
          onFontSizeChange={setFontSize}
          onNewAccount={() => setShowRegister(true)}
          secureMode={secureMode}
          currentUser={currentUser}
        />

        <Workspace fontSize={fontSize} />

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
