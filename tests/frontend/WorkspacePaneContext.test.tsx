import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { WorkspacePaneProvider, useWorkspacePane } from '@frontend/contexts/WorkspacePaneContext';

function TestConsumer() {
  const { activePane, setActivePane } = useWorkspacePane();
  return (
    <div>
      <span data-testid="pane">{activePane}</span>
      <button onClick={() => setActivePane('desktops')}>go desktops</button>
      <button onClick={() => setActivePane('terminals')}>go terminals</button>
    </div>
  );
}

describe('WorkspacePaneContext', () => {
  it('returns terminals as the default activePane', () => {
    render(
      <WorkspacePaneProvider>
        <TestConsumer />
      </WorkspacePaneProvider>,
    );
    expect(screen.getByTestId('pane').textContent).toBe('terminals');
  });

  it('updates activePane to desktops after setActivePane("desktops")', () => {
    render(
      <WorkspacePaneProvider>
        <TestConsumer />
      </WorkspacePaneProvider>,
    );
    fireEvent.click(screen.getByText('go desktops'));
    expect(screen.getByTestId('pane').textContent).toBe('desktops');
  });

  it('switches back to terminals after setActivePane("terminals")', () => {
    render(
      <WorkspacePaneProvider>
        <TestConsumer />
      </WorkspacePaneProvider>,
    );
    fireEvent.click(screen.getByText('go desktops'));
    expect(screen.getByTestId('pane').textContent).toBe('desktops');
    fireEvent.click(screen.getByText('go terminals'));
    expect(screen.getByTestId('pane').textContent).toBe('terminals');
  });

  it('useWorkspacePane returns context value from the provider', () => {
    render(
      <WorkspacePaneProvider>
        <TestConsumer />
      </WorkspacePaneProvider>,
    );
    // Initial render reflects provider default
    expect(screen.getByTestId('pane').textContent).toBe('terminals');
  });
});
