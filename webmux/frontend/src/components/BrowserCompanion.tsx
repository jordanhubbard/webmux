import { useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { api, type BrowserFrame, type BrowserAction } from '../utils/api';
import type { Session } from '../types';
import './BrowserCompanion.css';

/** A nonmodal drawer: the linked terminal remains interactive. */
export function BrowserCompanion({ session, navigation, onHide }: { session: Session; navigation?: { url: string } | null; onHide: () => void }) {
  const [frame, setFrame] = useState<BrowserFrame>();
  const [address, setAddress] = useState('');
  const [error, setError] = useState('');
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [paste, setPaste] = useState('');
  const [showPaste, setShowPaste] = useState(false);
  const mounted = useRef(true);
  const pending = useRef(false);
  const queue = useRef<Promise<void>>(Promise.resolve());
  const viewport = useRef<HTMLImageElement>(null);
  const addressEditing = useRef(false);

  const act = useCallback((action: BrowserAction) => {
    // Preserve keystroke order; a frame must never discard typed input.
    queue.current = queue.current.then(async () => {
      if (!mounted.current) return;
      pending.current = true;
      try {
        const result = await api.browserAction(session.id, action);
        if (!mounted.current) return;
        if (result.image) setFrame(result);
        if (!addressEditing.current && result.url && result.url !== 'about:blank') setAddress(result.url);
        setError(result.error ?? '');
        setReady(!result.error);
      } catch (err) {
        if (mounted.current) {
          setError(err instanceof Error ? err.message : 'Browser request failed');
          setReady(false);
        }
      } finally { pending.current = false; }
    });
    return queue.current;
  }, [session.id]);

  useEffect(() => {
    mounted.current = true;
    void act({ action: 'start' });
    return () => { mounted.current = false; };
  }, [act]);

  useEffect(() => {
    if (!navigation) return;
    addressEditing.current = false;
    setAddress(navigation.url);
    // The startup effect queues first, including when a link opens the drawer.
    void act({ action: 'navigate', url: navigation.url });
  }, [act, navigation]);

  useEffect(() => {
    if (!ready) return;
    const timer = window.setInterval(() => {
      if (!pending.current && document.visibilityState === 'visible') void act({ action: 'frame' });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [act, ready]);

  const point = (event: React.MouseEvent<HTMLImageElement> | React.WheelEvent<HTMLImageElement>) => {
    const bounds = event.currentTarget.getBoundingClientRect();
    return { x: (event.clientX - bounds.left) * 1100 / bounds.width, y: (event.clientY - bounds.top) * 760 / bounds.height };
  };

  const end = async () => {
    setBusy(true);
    setReady(false);
    await queue.current;
    try { await api.endBrowser(session.id); onHide(); }
    catch (err) { setError(err instanceof Error ? err.message : 'Could not end browser'); }
    finally { setBusy(false); }
  };

  return createPortal(
    <aside className="browser-companion" aria-label={`Browser for ${session.title}`} onKeyDown={e => e.stopPropagation()}>
      <header>
        <div><strong>Browser · {session.title}</strong><small>Runs on {session.transport === 'local' ? 'the WebMux server' : `${session.username ? `${session.username}@` : ''}${session.hostname}`} · private temporary profile</small></div>
        <button onClick={onHide}>Return to terminal</button>
        <button onClick={() => void end()} disabled={busy}>End browser</button>
      </header>
      <form className="browser-navigation" onSubmit={e => { e.preventDefault(); addressEditing.current = false; void act({ action: 'navigate', url: address.trim() }); viewport.current?.focus(); }}>
        <button type="button" aria-label="Browser back" disabled={!ready} onClick={() => void act({ action: 'back' })}>←</button>
        <button type="button" aria-label="Reload browser page" disabled={!ready} onClick={() => void act({ action: 'reload' })}>↻</button>
        <input aria-label="Remote browser URL" placeholder="Paste the authentication URL from this terminal" value={address} onFocus={() => { addressEditing.current = true; }} onBlur={() => { addressEditing.current = false; }} onChange={e => setAddress(e.target.value)} />
        <button type="submit" disabled={busy}>Go</button>
      </form>
      {frame && frame.tabs.length > 1 && <label className="browser-tabs">Page <select aria-label="Remote browser page" value={frame.target} onChange={e => void act({ action: 'tab', target: e.target.value })}>
        {frame.tabs.map(tab => <option key={tab.id} value={tab.id}>{tab.title || 'Untitled page'}</option>)}
      </select></label>}
      {error ? <div role="alert" className="browser-message">{error} <button onClick={() => void act({ action: 'start' })}>Reconnect browser</button></div>
        : !frame && <div role="status" className="browser-message">Starting browser on the terminal host…</div>}
      <div className="browser-viewport">
        {frame?.image && <img ref={viewport} src={`data:image/jpeg;base64,${frame.image}`} alt="Interactive remote browser viewport" tabIndex={0}
          draggable={false}
          onClick={e => { e.currentTarget.focus(); void act({ action: 'click', ...point(e) }); }}
          onWheel={e => { e.stopPropagation(); void act({ action: 'scroll', ...point(e), deltaY: Math.max(-3000, Math.min(3000, e.deltaY)) }); }}
          onKeyDown={e => {
            // Escape returns focus to local controls; Tab navigates the remote page.
            if (e.key === 'Escape') { e.currentTarget.blur(); return; }
            if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'v') return;
            e.preventDefault();
            void act({ action: 'key', text: e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey ? e.key : undefined, key: e.key, code: e.code, keyCode: e.keyCode, modifiers: (e.altKey ? 1 : 0) | (e.ctrlKey ? 2 : 0) | (e.metaKey ? 4 : 0) | (e.shiftKey ? 8 : 0) });
          }}
          onPaste={e => { e.preventDefault(); void act({ action: 'text', text: e.clipboardData.getData('text/plain') }); }}
        />}
      </div>
      <footer>
        <span>Click the page to type. Esc releases keyboard focus. Hiding keeps cookies; ending clears them.</span>
        <button onClick={() => setShowPaste(value => !value)} disabled={!ready}>Paste text</button>
      </footer>
      {showPaste && <form className="browser-navigation" onSubmit={e => { e.preventDefault(); void act({ action: 'text', text: paste }); setPaste(''); setShowPaste(false); viewport.current?.focus(); }}>
        <input aria-label="Text to paste into remote browser" value={paste} onChange={e => setPaste(e.target.value)} autoComplete="off" />
        <button type="submit">Send to page</button>
      </form>}
    </aside>, document.body,
  );
}
