import type { RefObject } from 'react';
import type { TerminalHandle } from './Terminal';
import './TerminalActions.css';

interface TerminalActionsProps {
  terminalRef: RefObject<TerminalHandle | null>;
  transcriptEnabled: boolean;
  connected: boolean;
}

/** Explicit controls leave every terminal key combination available to the PTY. */
export function TerminalActions({ terminalRef, transcriptEnabled, connected }: TerminalActionsProps) {
  return (
    <div className="terminal-actions">
      <button type="button" aria-label="Search terminal" title="Search terminal scrollback"
        onClick={() => terminalRef.current?.openSearch()}>
        <svg width="14" height="14" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
          <circle cx="8" cy="8" r="5" /><path d="m12 12 5 5" />
        </svg>
      </button>
      <button type="button" aria-label="Log session" aria-pressed={transcriptEnabled}
        title={transcriptEnabled ? 'Pause session transcript logging' : 'Start session transcript logging'}
        disabled={!connected} onClick={() => terminalRef.current?.toggleTranscript()}>
        <span aria-hidden="true">{transcriptEnabled ? '●' : '○'}</span> Log
      </button>
    </div>
  );
}
