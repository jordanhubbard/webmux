import { useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { api, type BrowserFrame, type BrowserAction } from '../utils/api';
import { copyDesktopText } from '../utils/terminalClipboard';
import { handoffLink } from '../utils/authHandoff';
import { Dialog } from './Dialog';
import type { Session } from '../types';
import './BrowserCompanion.css';

const MAX_FILE_BYTES = 10 * 1024 * 1024;
const NAVIGATION_ACTIONS = new Set<BrowserAction['action']>(['start', 'navigate', 'back', 'forward', 'reload', 'tab', 'new_tab', 'close_tab']);
const modifiers = (event: { altKey: boolean; ctrlKey: boolean; metaKey: boolean; shiftKey: boolean }) =>
  (event.altKey ? 1 : 0) | (event.ctrlKey ? 2 : 0) | (event.metaKey ? 4 : 0) | (event.shiftKey ? 8 : 0);

function encodedFile(file: File): Promise<{ name: string; data: string }> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error(`Cannot read ${file.name}`));
    reader.onload = () => resolve({ name: file.name, data: String(reader.result).split(',')[1] });
    reader.readAsDataURL(file);
  });
}

function saveDownload(file: { name: string; data: string }) {
  const bytes = Uint8Array.from(atob(file.data), character => character.charCodeAt(0));
  const url = URL.createObjectURL(new Blob([bytes], { type: 'application/octet-stream' }));
  const link = document.createElement('a'); link.href = url; link.download = file.name; link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 30_000);
}

/** A normal browser workspace backed by Chromium on the terminal's host. */
export function BrowserCompanion({ session, navigation, onHide, onAuthentication }: {
  session: Session; navigation?: { url: string } | null; onHide: () => void; onAuthentication?: (url: string) => void;
}) {
  const [frame, setFrame] = useState<BrowserFrame>();
  const [address, setAddress] = useState('');
  const [error, setError] = useState('');
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [tabBusy, setTabBusy] = useState(false);
  const [paste, setPaste] = useState('');
  const [prompt, setPrompt] = useState('');
  const [showPaste, setShowPaste] = useState(false);
  const [showFind, setShowFind] = useState(false);
  const [findText, setFindText] = useState('');
  const [findMessage, setFindMessage] = useState('');
  const findInput = useRef<HTMLInputElement>(null);
  const [expanded, setExpanded] = useState(false);
  const mounted = useRef(true);
  const pending = useRef(0);
  const queue = useRef<Promise<void>>(Promise.resolve());
  const coalesced = useRef<{ action: BrowserAction } | null>(null);
  const viewport = useRef<HTMLImageElement>(null);
  const keyboard = useRef<HTMLTextAreaElement>(null);
  const composing = useRef(false);
  const viewportBox = useRef<HTMLDivElement>(null);
  const addressInput = useRef<HTMLInputElement>(null);
  const addressEditing = useRef(false);
  // Go blurs the input before submitting. Keep an edited draft through that
  // interval so an in-flight frame cannot replace the user's destination.
  const addressDraft = useRef(false);
  const lastClick = useRef({ time: 0, x: 0, y: 0, button: -1, count: 0 });
  const dragging = useRef<{ button: string; count: number } | null>(null);
  const lastMove = useRef(0);

  const act = useCallback((action: BrowserAction) => {
    if (NAVIGATION_ACTIONS.has(action.action)) {
      addressDraft.current = false;
      addressEditing.current = false;
    }
    // Coalesce queued pointer motion only within one down/up segment. Key and
    // button events keep their order, even when HTTP is slower than the mouse.
    if (action.action === 'mouse_move' && coalesced.current) {
      coalesced.current.action = action; return queue.current;
    }
    const item = { action };
    coalesced.current = action.action === 'mouse_move' ? item : null;
    pending.current++;
    queue.current = queue.current.then(async () => {
      if (coalesced.current === item) coalesced.current = null;
      try {
        if (!mounted.current) return;
        const result = await api.browserAction(session.id, item.action);
        if (!mounted.current) return;
        if (!result.error && result.protocol !== 2) throw new Error('Update WebMux on the terminal host before reopening this browser; its browser worker uses an older protocol.');
        const { file, clipboard, ...metadata } = result;
        setFrame(previous => ({ ...metadata, url: metadata.url || previous?.url || '', target: metadata.target || previous?.target || '', tabs: metadata.tabs ?? previous?.tabs ?? [],
          width: metadata.width || previous?.width, height: metadata.height || previous?.height,
          image: metadata.image || (!metadata.target || previous?.target === metadata.target ? previous?.image : undefined) }));
        if (!addressEditing.current && !addressDraft.current && result.url && result.url !== 'about:blank') setAddress(result.url);
        else if (!addressEditing.current && !addressDraft.current && result.url === 'about:blank') setAddress('');
        setError(result.error ?? '');
        setReady(previous => previous || !result.error);
        if (clipboard !== undefined && !await copyDesktopText(clipboard)) setError('The browser blocked clipboard access for this site.');
        if (file) saveDownload(file);
        if (typeof result.found === 'boolean') setFindMessage(result.found ? 'Match found.' : 'No matches on this page.');
      } catch (err) {
        if (mounted.current) {
          setError(err instanceof Error ? err.message : 'Browser request failed');
          if (item.action.action === 'start') setReady(false);
        }
      } finally { pending.current--; }
    });
    return queue.current;
  }, [session.id]);

  useEffect(() => {
    mounted.current = true; void act({ action: 'start' });
    return () => { mounted.current = false; };
  }, [act]);

  useEffect(() => {
    if (!navigation) return;
    addressEditing.current = false; setAddress(navigation.url);
    void act({ action: 'navigate', url: navigation.url });
  }, [act, navigation]);

  useEffect(() => {
    if (!ready) return;
    const timer = window.setInterval(() => {
      if (pending.current === 0 && document.visibilityState === 'visible') void act({ action: 'frame' });
    }, 500);
    return () => window.clearInterval(timer);
  }, [act, ready]);

  useEffect(() => {
    if (!ready || !viewportBox.current || typeof ResizeObserver === 'undefined') return;
    let timer: ReturnType<typeof setTimeout>;
    let size = '';
    const observer = new ResizeObserver(entries => {
      const { width, height } = entries[0].contentRect;
      const w = Math.max(320, Math.min(2560, Math.round(width)));
      const h = Math.max(240, Math.min(1600, Math.round(height)));
      if (`${w}x${h}` === size) return;
      clearTimeout(timer);
      timer = setTimeout(() => { size = `${w}x${h}`; void act({ action: 'resize', width: w, height: h }); }, 120);
    });
    observer.observe(viewportBox.current);
    return () => { clearTimeout(timer); observer.disconnect(); };
  }, [act, ready]);

  useEffect(() => { if (showFind) findInput.current?.focus(); }, [showFind]);

  useEffect(() => { setPrompt(frame?.dialog?.prompt ?? ''); }, [frame?.dialog?.message, frame?.dialog?.prompt]);

  const point = (event: React.PointerEvent<HTMLImageElement> | React.WheelEvent<HTMLImageElement>) => {
    const bounds = event.currentTarget.getBoundingClientRect();
    return { x: Math.max(0, Math.min(frame?.width ?? 1100, (event.clientX - bounds.left) * (frame?.width ?? 1100) / bounds.width)),
      y: Math.max(0, Math.min(frame?.height ?? 760, (event.clientY - bounds.top) * (frame?.height ?? 760) / bounds.height)) };
  };
  const end = async () => {
    setBusy(true); setReady(false); await queue.current;
    try { await api.endBrowser(session.id); onHide(); }
    catch (err) { setError(err instanceof Error ? err.message : 'Could not end browser'); }
    finally { setBusy(false); }
  };
  const external = handoffLink(frame?.url ?? '');
  const changeTab = (action: BrowserAction) => {
    setTabBusy(true);
    void act(action).finally(() => {
      setTabBusy(false);
      if (action.action === 'new_tab') addressInput.current?.focus();
    });
  };
  const upload = async (files: FileList | null) => {
    const selected = Array.from(files ?? []);
    if (!selected.length) return;
    if (selected.length > 8 || selected.reduce((sum, file) => sum + file.size, 0) > MAX_FILE_BYTES) { setError('Choose at most eight files totaling 10 MiB.'); return; }
    setBusy(true);
    try { await act({ action: 'upload', files: await Promise.all(selected.map(encodedFile)) }); }
    catch (err) { setError(String(err)); }
    finally { setBusy(false); }
  };

  return createPortal(
    <aside className={`browser-companion${expanded ? ' browser-expanded' : ''}`} aria-label={`Browser for ${session.title}`} onKeyDown={event => event.stopPropagation()}>
      <header>
        <div><strong>Browser · {session.title}</strong><small>Runs on {session.transport === 'local' || session.browser_local ? 'the WebMux server' : `${session.username ? `${session.username}@` : ''}${session.hostname}`} · private temporary profile</small></div>
        <button onClick={() => setExpanded(value => !value)} aria-expanded={expanded}>{expanded ? 'Restore browser size' : 'Expand browser'}</button>
        <button onClick={onHide}>Return to terminal</button>
        <button onClick={() => void end()} disabled={busy}>End browser</button>
      </header>
      <form className="browser-navigation" onSubmit={event => {
        event.preventDefault(); addressEditing.current = false;
        if (!address.trim()) return;
        const url = /^https?:\/\//i.test(address.trim()) ? address.trim() : `https://${address.trim()}`;
        void act({ action: 'navigate', url }); viewport.current?.focus();
      }}>
        <button type="button" aria-label="Browser back" disabled={!ready || frame?.canBack === false} onClick={() => void act({ action: 'back' })}>←</button>
        <button type="button" aria-label="Browser forward" disabled={!ready || !frame?.canForward} onClick={() => void act({ action: 'forward' })}>→</button>
        <button type="button" aria-label="Stop browser page" disabled={!ready} onClick={() => void act({ action: 'stop' })}>×</button>
        <button type="button" aria-label="Reload browser page" disabled={!ready} onClick={() => void act({ action: 'reload' })}>↻</button>
        <input ref={addressInput} aria-label="Remote browser URL" placeholder="Enter a website URL" value={address} onFocus={() => { addressEditing.current = true; }} onBlur={() => { addressEditing.current = false; }}
          onChange={event => { addressDraft.current = true; setAddress(event.target.value); }}
          onKeyDown={event => {
            if (event.key !== 'Escape') return;
            event.preventDefault(); event.stopPropagation();
            addressDraft.current = false; addressEditing.current = false;
            setAddress(frame?.url === 'about:blank' ? '' : frame?.url ?? '');
            event.currentTarget.blur();
          }} />
        <button type="submit" disabled={busy}>Go</button>
      </form>
      <div className="browser-tabs">
        {frame && frame.tabs.length > 0 && <label>Tab <select aria-label="Remote browser page" value={frame.target} disabled={tabBusy} onChange={event => changeTab({ action: 'tab', target: event.target.value })}>
          {frame.tabs.map(tab => <option key={tab.id} value={tab.id}>{tab.title || 'New tab'}</option>)}
        </select></label>}
        <button onClick={() => changeTab({ action: 'new_tab' })} disabled={!ready || tabBusy}>New tab</button>
        <button onClick={() => changeTab({ action: 'close_tab', target: frame?.target })} disabled={!ready || tabBusy || (frame?.tabs.length ?? 0) < 2}>Close tab</button>
        {external && !external.loopback && !external.relay && <a href={external.url} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">Open in your browser</a>}
        {external && onAuthentication && <button onClick={() => onAuthentication(external.url)}>Sign-in handoff</button>}
      </div>
      {frame?.url.includes('/sessions/two-factor/webauthn') && <div role="note" className="browser-message">Use “Sign-in handoff” for device-local passkeys, security keys and existing sign-in sessions.</div>}
      {error ? <div role="alert" className="browser-message">{error} <button onClick={() => void act({ action: 'start' })}>Reconnect browser</button></div>
        : !frame && <div role="status" className="browser-message">Starting browser on the terminal host…</div>}
      <div className="browser-viewport" ref={viewportBox}>
        {frame?.image && <img ref={viewport} src={`data:image/jpeg;base64,${frame.image}`} alt="Interactive remote browser viewport" tabIndex={0}
          draggable={false} style={{ pointerEvents: tabBusy ? 'none' : undefined }}
          onPointerDown={event => {
            event.preventDefault(); keyboard.current?.focus({ preventScroll: true }); event.currentTarget.setPointerCapture(event.pointerId);
            const p = point(event); const previous = lastClick.current;
            const count = event.button === previous.button && performance.now() - previous.time < 450 && Math.abs(p.x - previous.x) < 5 && Math.abs(p.y - previous.y) < 5 ? previous.count % 3 + 1 : 1;
            lastClick.current = { ...p, time: performance.now(), button: event.button, count };
            const button = ['left', 'middle', 'right'][event.button] ?? 'left'; dragging.current = { button, count };
            void act({ action: 'mouse_down', ...p, button, buttons: event.buttons || 1, clickCount: count, modifiers: modifiers(event) });
          }}
          onPointerMove={event => {
            if (performance.now() - lastMove.current < 70) return; lastMove.current = performance.now();
            void act({ action: 'mouse_move', ...point(event), button: dragging.current?.button ?? 'none', buttons: event.buttons, modifiers: modifiers(event) });
          }}
          onPointerUp={event => {
            const held = dragging.current; if (!held) return; dragging.current = null;
            void act({ action: 'mouse_move', ...point(event), button: held.button, buttons: event.buttons | (held.button === 'right' ? 2 : held.button === 'middle' ? 4 : 1), modifiers: modifiers(event) });
            void act({ action: 'mouse_up', ...point(event), button: held.button, buttons: event.buttons, clickCount: held.count, modifiers: modifiers(event) });
          }}
          onPointerCancel={event => {
            if (dragging.current) { void act({ action: 'mouse_up', ...point(event), button: dragging.current.button, buttons: 0, clickCount: dragging.current.count }); dragging.current = null; }
          }}
          onContextMenu={event => event.preventDefault()}
          onWheel={event => { event.stopPropagation(); void act({ action: 'scroll', ...point(event), deltaX: Math.max(-3000, Math.min(3000, event.deltaX)), deltaY: Math.max(-3000, Math.min(3000, event.deltaY)), modifiers: modifiers(event) }); }}
          onFocus={() => keyboard.current?.focus({ preventScroll: true })}
        />}
        <textarea ref={keyboard} disabled={tabBusy} className="browser-keyboard" aria-label="Type into remote page" tabIndex={-1} autoCapitalize="off" autoCorrect="off" spellCheck={false}
          onCompositionStart={() => { composing.current = true; }}
          onCompositionEnd={event => { composing.current = false; if (event.data) void act({ action: 'text', text: event.data }); event.currentTarget.value = ''; }}
          onInput={event => { if (!composing.current && event.currentTarget.value) { void act({ action: 'text', text: event.currentTarget.value }); event.currentTarget.value = ''; } }}
          onKeyDown={event => {
            if (composing.current || event.nativeEvent.isComposing || event.key === 'Process' || event.key === 'Unidentified') return;
            if (event.key === 'Escape' && event.shiftKey) { event.currentTarget.blur(); addressInput.current?.focus(); return; }
            if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'v') return;
            event.preventDefault();
            if ((event.ctrlKey || event.metaKey) && !event.altKey) {
              const key = event.key.toLowerCase();
              if (key === 'f' && !event.shiftKey) { setShowFind(true); findInput.current?.focus(); return; }
              if (key === 'r') { void act({ action: 'reload', ignoreCache: event.shiftKey }); return; }
              if (key === 't' && !event.shiftKey) { if (!tabBusy) changeTab({ action: 'new_tab' }); return; }
              if (key === 'w' && !event.shiftKey) {
                if (!tabBusy) { if ((frame?.tabs.length ?? 0) > 1) changeTab({ action: 'close_tab', target: frame?.target }); else onHide(); }
                return;
              }
            }
            if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'c') { void act({ action: 'copy' }); return; }
            if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'l') { addressInput.current?.focus(); addressInput.current?.select(); return; }
            void act({ action: 'key', text: event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey ? event.key : undefined, key: event.key, code: event.code, keyCode: event.keyCode, modifiers: modifiers(event) });
          }}
          onPaste={event => { event.preventDefault(); void act({ action: 'text', text: event.clipboardData.getData('text/plain') }); }}
        />
      </div>
      <footer>
        <span>Shift+Esc releases focus. Hiding keeps cookies; ending clears this private browser.</span>
        <button onClick={() => setShowFind(value => !value)} disabled={!ready}>Find text</button>
        <button onClick={() => void act({ action: 'copy' })} disabled={!ready}>Copy selection</button>
        <button onClick={() => setShowPaste(value => !value)} disabled={!ready}>Paste text</button>
        <details><summary>Downloads ({frame?.downloads?.length ?? 0})</summary>
          {frame?.downloads?.map(file => <div key={file.id}>{file.name} · {file.state} · {Math.ceil(file.bytes / 1024)} KiB <button disabled={file.state !== 'completed' || busy} onClick={() => void act({ action: 'download', target: file.id })}>Save {file.name}</button></div>)}
        </details>
      </footer>
      {showFind && <form className="browser-navigation browser-find" onSubmit={event => { event.preventDefault(); void act({ action: 'find', text: findText }); }}>
        <input ref={findInput} aria-label="Find on page" value={findText} onChange={event => { setFindText(event.target.value); setFindMessage(''); }} />
        <button type="button" disabled={!findText} onClick={() => void act({ action: 'find', text: findText, backward: true })}>Previous match</button>
        <button type="submit" disabled={!findText}>Next match</button>
        <button type="button" onClick={() => { setShowFind(false); keyboard.current?.focus(); }}>Close find</button>
        {findMessage && <span role="status">{findMessage}</span>}
      </form>}
      {showPaste && <form className="browser-navigation" onSubmit={event => { event.preventDefault(); void act({ action: 'text', text: paste }); setPaste(''); setShowPaste(false); viewport.current?.focus(); }}>
        <input aria-label="Text to paste into remote browser" value={paste} onChange={event => setPaste(event.target.value)} autoComplete="off" />
        <button type="submit">Send to page</button>
      </form>}
      {frame?.dialog && <Dialog title={`Browser ${frame.dialog.type}`} onClose={() => void act({ action: 'dialog', target: frame.target, accept: false })}>
        <p>{frame.dialog.message}</p>
        {frame.dialog.type === 'prompt' && <input aria-label="Browser prompt response" data-dialog-autofocus value={prompt} onChange={event => setPrompt(event.target.value)} />}
        <button onClick={() => void act({ action: 'dialog', target: frame.target, accept: true, text: prompt })}>OK</button>
        {frame.dialog.type !== 'alert' && <button onClick={() => void act({ action: 'dialog', target: frame.target, accept: false })}>Cancel</button>}
      </Dialog>}
      {frame?.select && <Dialog title="Choose a page option" onClose={() => void act({ action: 'cancel_select', target: frame.target })}>
        <label>Option <select aria-label="Page option" data-dialog-autofocus value={frame.select.options.find(option => option.selected)?.value ?? ''} onChange={event => void act({ action: 'select_option', target: frame.target, text: event.target.value })}>
          {frame.select.options.map((option, index) => <option key={index} value={option.value} disabled={option.disabled}>{option.label}</option>)}
        </select></label>
        <button onClick={() => void act({ action: 'cancel_select', target: frame.target })}>Cancel</button>
      </Dialog>}
      {frame?.fileChooser && <Dialog title="Upload files to this page" onClose={() => void act({ action: 'cancel_upload' })}>
        <p>Choose {frame.fileChooser.multiple ? 'files' : 'a file'} from your computer. Up to 10 MiB per selection.</p>
        <input aria-label="Choose files for page" type="file" multiple={frame.fileChooser.multiple} disabled={busy} onChange={event => void upload(event.target.files)} />
        <button onClick={() => void act({ action: 'cancel_upload' })}>Cancel</button>
      </Dialog>}
    </aside>, document.body,
  );
}
