import { useEffect, useRef, useState } from 'react';
import RFB from '@novnc/novnc';
import { buildVncWsUrl } from '../utils/api';

interface VncViewerProps {
  sessionId: string;
  vncUsername?: string;
  vncPassword?: string;
  mode: 'thumbnail' | 'fullscreen';
  onStateChange?: (state: 'connecting' | 'connected' | 'disconnected' | 'error') => void;
  onClipboard?: (text: string) => void;
  rfbRef?: React.MutableRefObject<VncClientControl | null>;
}

export interface VncClientControl {
  clipboardPasteFrom(text: string): void;
  sendCtrlAltDel(): void;
}

export function VncViewer({ sessionId, vncUsername, vncPassword, mode, onStateChange, rfbRef, onClipboard }: VncViewerProps) {
  const clipboardRef = useRef(onClipboard);
  clipboardRef.current = onClipboard;
  const containerRef = useRef<HTMLDivElement>(null);
  const [credentialError, setCredentialError] = useState<string | null>(null);

  // Keep latest callbacks/values in refs so RFB event listeners never capture stale closures.
  const onStateChangeRef = useRef(onStateChange);
  useEffect(() => { onStateChangeRef.current = onStateChange; }, [onStateChange]);
  const vncPasswordRef = useRef(vncPassword);
  useEffect(() => { vncPasswordRef.current = vncPassword; }, [vncPassword]);

  useEffect(() => {
    setCredentialError(null);

    if (!window.isSecureContext) {
      onStateChangeRef.current?.('error');
      return;
    }
    if (!containerRef.current) return;

    onStateChangeRef.current?.('connecting');

    const options = { credentials: { username: vncUsername, ...(vncPassword ? { password: vncPassword } : {}) } };
    const rfb = new RFB(containerRef.current, buildVncWsUrl(sessionId), options);

    rfb.scaleViewport = mode === 'fullscreen';
    rfb.resizeSession = false;

    rfb.addEventListener('clipboard', event => {
      clipboardRef.current?.(event.detail.text.slice(0, 128 * 1024));
    });

    rfb.addEventListener('connect', () => {
      onStateChangeRef.current?.('connected');
    });

    rfb.addEventListener('disconnect', () => {
      onStateChangeRef.current?.('disconnected');
    });

    rfb.addEventListener('credentialsrequired', event => {
      if (event.detail.types.includes('username') && !vncUsername) {
        setCredentialError('This server requires a VNC username. Create a new connection with the username filled in.');
        rfb.disconnect();
        onStateChangeRef.current?.('error');
        return;
      }
      rfb.sendCredentials({ username: vncUsername, password: vncPasswordRef.current || '' });
    });

    rfb.addEventListener('securityfailure', () => {
      onStateChangeRef.current?.('error');
    });

    if (rfbRef) {
      rfbRef.current = rfb;
    }

    return () => {
      rfb.disconnect();
      if (rfbRef) {
        rfbRef.current = null;
      }
    };
    // Re-run when the session or display mode changes.
  }, [sessionId, mode, vncUsername]);

  if (!window.isSecureContext) {
    return <div role="alert" style={{ padding: 16 }}>VNC requires HTTPS or localhost. Open WebMux over HTTPS or through a localhost SSH tunnel.</div>;
  }

  const errorNotice = credentialError && <div role="alert" style={{ position: 'absolute', inset: 0, padding: 16 }}>{credentialError}</div>;

  if (mode === 'thumbnail') {
    return (
      <div style={{ width: '100%', height: '100%', overflow: 'hidden', position: 'relative' }}>
        <div
          ref={containerRef}
          data-1p-ignore
          style={{
            position: 'absolute',
            top: 0,
            left: 0,
            width: '400%',
            height: '400%',
            transform: 'scale(0.25)',
            transformOrigin: '0 0',
            pointerEvents: 'none',
          }}
        />
        {errorNotice}
      </div>
    );
  }

  return (
    <div style={{ position: 'relative', width: '100%', height: '100%' }}>
      <div ref={containerRef} data-1p-ignore style={{ width: '100%', height: '100%' }} />
      {errorNotice}
    </div>
  );
}
