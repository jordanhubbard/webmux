import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import type { WorkspaceName } from '../types';

export type WorkspacePane = WorkspaceName;

export const DEFAULT_WORKSPACE_PANE: WorkspacePane = 'terminals';

interface WorkspacePaneContextValue {
  activePane: WorkspacePane;
  setActivePane: (pane: WorkspacePane) => void;
}

const WorkspacePaneContext = createContext<WorkspacePaneContextValue>({
  activePane: DEFAULT_WORKSPACE_PANE,
  setActivePane: () => {},
});

export function WorkspacePaneProvider({
  children,
  defaultPane = DEFAULT_WORKSPACE_PANE,
  availablePanes = ['terminals', 'desktops'],
}: {
  children: ReactNode;
  defaultPane?: WorkspacePane;
  availablePanes?: WorkspacePane[];
}) {
  const availableKey = availablePanes.join('\0');
  const available = useMemo(() => new Set(availablePanes), [availableKey]);
  const [activePane, setActivePaneState] = useState<WorkspacePane>(defaultPane);
  const userSelectedPane = useRef(false);

  useEffect(() => {
    if (!available.has(activePane)) {
      setActivePaneState(available.has(defaultPane) ? defaultPane : DEFAULT_WORKSPACE_PANE);
      return;
    }
    if (!userSelectedPane.current && activePane !== defaultPane && available.has(defaultPane)) {
      setActivePaneState(defaultPane);
    }
  }, [activePane, available, defaultPane]);

  const setActivePane = useCallback((pane: WorkspacePane) => {
    userSelectedPane.current = true;
    setActivePaneState(pane);
  }, []);

  return (
    <WorkspacePaneContext.Provider value={{ activePane, setActivePane }}>
      {children}
    </WorkspacePaneContext.Provider>
  );
}

export function useWorkspacePane() {
  return useContext(WorkspacePaneContext);
}
