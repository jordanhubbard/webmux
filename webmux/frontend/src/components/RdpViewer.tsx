import { useEffect, useRef } from 'react';
import Guacamole from 'guacamole-common-js';
import { buildRdpWsUrl } from '../utils/api';

interface RdpViewerProps {
  sessionId: string;
  mode: 'thumbnail' | 'fullscreen';
  onStateChange?: (state: 'connecting' | 'connected' | 'disconnected' | 'error') => void;
  clientRef?: React.MutableRefObject<RdpClientControl | null>;
}

export type RdpClientControl = Pick<Guacamole.Client, 'createClipboardStream' | 'sendKeyEvent'>;

export function RdpViewer({ sessionId, mode, onStateChange, clientRef }: RdpViewerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const onStateChangeRef = useRef(onStateChange);
  useEffect(() => { onStateChangeRef.current = onStateChange; }, [onStateChange]);

  useEffect(() => {
    if (!containerRef.current) return;

    onStateChangeRef.current?.('connecting');

    const tunnel = new Guacamole.WebSocketTunnel(buildRdpWsUrl(sessionId));
    const client = new Guacamole.Client(tunnel);

    const displayElem = client.getDisplay().getElement();
    containerRef.current.appendChild(displayElem);

    // Guacamole.Client.State: IDLE=0, CONNECTING=1, WAITING=2, CONNECTED=3, DISCONNECTING=4, DISCONNECTED=5
    client.onstatechange = (state: number) => {
      if (state === 3) onStateChangeRef.current?.('connected');
      else if (state === 5) onStateChangeRef.current?.('disconnected');
    };

    client.onerror = () => {
      onStateChangeRef.current?.('error');
    };

    let active = true;

    if (mode === 'fullscreen') {
      const mouse = new Guacamole.Mouse(containerRef.current);
      mouse.onmousedown = state => { if (active) client.sendMouseState(state); };
      mouse.onmouseup = state => { if (active) client.sendMouseState(state); };
      mouse.onmousemove = state => { if (active) client.sendMouseState(state); };

      const keyboard = new Guacamole.Keyboard(document);
      keyboard.onkeydown = (keysym: number) => { if (active) client.sendKeyEvent(1, keysym); };
      keyboard.onkeyup = (keysym: number) => { if (active) client.sendKeyEvent(0, keysym); };
    }

    if (clientRef) clientRef.current = client;

    client.connect();

    return () => {
      active = false;
      client.disconnect();
      if (containerRef.current && displayElem.parentNode === containerRef.current) {
        containerRef.current.removeChild(displayElem);
      }
      if (clientRef) clientRef.current = null;
    };
  }, [sessionId, mode]);

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
            transform: 'scale(0.3)',
            transformOrigin: '0 0',
            pointerEvents: 'none',
          }}
        />
      </div>
    );
  }

  return (
    <div
      ref={containerRef}
      data-1p-ignore
      // Guacamole places canvas layers at z-index -1. Keep them above this
      // viewer's background while preserving their ordering below child layers.
      style={{ width: '100%', height: '100%', overflow: 'auto', background: '#000', isolation: 'isolate' }}
      tabIndex={0}
    />
  );
}
