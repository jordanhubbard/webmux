import { useEffect, useRef } from 'react';
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
import RFB from '@novnc/novnc/lib/rfb.js';
import { buildVncWsUrl } from '../utils/api';

interface VncViewerProps {
  sessionId: string;
  vncPassword?: string;
  mode: 'thumbnail' | 'fullscreen';
  onStateChange?: (state: 'connecting' | 'connected' | 'disconnected' | 'error') => void;
  rfbRef?: React.MutableRefObject<any>;
}

export function VncViewer({ sessionId, vncPassword, mode, onStateChange, rfbRef }: VncViewerProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  // Keep latest callbacks/values in refs so RFB event listeners never capture stale closures.
  const onStateChangeRef = useRef(onStateChange);
  useEffect(() => { onStateChangeRef.current = onStateChange; }, [onStateChange]);
  const vncPasswordRef = useRef(vncPassword);
  useEffect(() => { vncPasswordRef.current = vncPassword; }, [vncPassword]);

  useEffect(() => {
    if (!containerRef.current) return;

    onStateChangeRef.current?.('connecting');

    const options = vncPassword ? { credentials: { password: vncPassword } } : undefined;
    const rfb = new RFB(containerRef.current, buildVncWsUrl(sessionId), options);

    rfb.scaleViewport = mode === 'fullscreen';
    rfb.resizeSession = false;

    rfb.addEventListener('connect', () => {
      onStateChangeRef.current?.('connected');
    });

    rfb.addEventListener('disconnect', () => {
      onStateChangeRef.current?.('disconnected');
    });

    rfb.addEventListener('credentialsrequired', () => {
      rfb.sendCredentials({ password: vncPasswordRef.current || '' });
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, mode]);

  if (mode === 'thumbnail') {
    return (
      <div style={{ width: '100%', height: '100%', overflow: 'hidden', position: 'relative' }}>
        <div
          ref={containerRef}
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
      </div>
    );
  }

  return (
    <div ref={containerRef} style={{ width: '100%', height: '100%' }} />
  );
}
