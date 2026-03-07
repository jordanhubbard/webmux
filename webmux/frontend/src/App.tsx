import { useState, useEffect, useCallback } from 'react';
import { useAuth } from './hooks/useAuth';
import { LoginPage } from './components/LoginPage';
import { TopBar } from './components/TopBar';
import { Workspace } from './components/Workspace';
import { InputBroadcastProvider } from './contexts/InputBroadcastContext';
import { api } from './utils/api';

export default function App() {
  const auth = useAuth();
  const [fontSize, setFontSize] = useState(14);
  const [showAddDialog, setShowAddDialog] = useState(false);
  const [secureMode, setSecureMode] = useState(true);

  useEffect(() => {
    api.getConfig().then(config => {
      setSecureMode(config.app.secure_mode);
      setFontSize(config.app.default_term.font_size);
    }).catch(() => {});
  }, []);

  const handleDialogClose = useCallback(() => {
    setShowAddDialog(false);
  }, []);

  if (auth.isLoading) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#888' }}>
        Loading…
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
          onAddSession={() => setShowAddDialog(true)}
          secureMode={secureMode}
        />

        <Workspace
          fontSize={fontSize}
          showAddDialog={showAddDialog}
          onDialogClose={handleDialogClose}
        />
      </div>
    </InputBroadcastProvider>
  );
}
