import { createContext, useContext, useState } from 'react';
import type { ReactNode } from 'react';

export type WorkspacePane = 'terminals' | 'desktops';

interface WorkspacePaneContextValue {
  activePane: WorkspacePane;
  setActivePane: (pane: WorkspacePane) => void;
}

const WorkspacePaneContext = createContext<WorkspacePaneContextValue>({
  activePane: 'terminals',
  setActivePane: () => {},
});

export function WorkspacePaneProvider({ children }: { children: ReactNode }) {
  const [activePane, setActivePane] = useState<WorkspacePane>('terminals');
  return (
    <WorkspacePaneContext.Provider value={{ activePane, setActivePane }}>
      {children}
    </WorkspacePaneContext.Provider>
  );
}

export function useWorkspacePane() {
  return useContext(WorkspacePaneContext);
}
