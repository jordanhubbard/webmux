import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState } from 'react';
import type { CSSProperties, KeyboardEvent, PointerEvent } from 'react';
import { BROWSER_PANE_MOBILE_WIDTH, browserPaneRect, fitBrowserPaneRect, readBrowserPaneLayout, saveBrowserPaneLayout } from './browserPaneLayout';
import type { BrowserPaneLayout, BrowserPaneRect } from './browserPaneLayout';

const ACTIVE_PANE_EVENT = 'webmux:browser-pane-focus';
type DragKind = 'move' | 'resize';

export function useBrowserPaneLayout(expanded: boolean) {
  const id = useId();
  const [layout, setLayout] = useState(readBrowserPaneLayout);
  const [viewport, setViewport] = useState(() => ({ width: window.innerWidth, height: window.innerHeight }));
  const [active, setActive] = useState(true);
  const latest = useRef(layout);
  const changed = useRef(false);
  const drag = useRef<{ kind: DragKind; pointer: number; x: number; y: number; rect: BrowserPaneRect; mode: BrowserPaneLayout['mode'] } | null>(null);
  const narrow = viewport.width <= BROWSER_PANE_MOBILE_WIDTH;
  const rect = browserPaneRect(layout, viewport);
  useEffect(() => { drag.current = null; }, [expanded, narrow, layout.mode, viewport.width, viewport.height]);

  useLayoutEffect(() => { latest.current = layout; }, [layout]);
  useEffect(() => {
    const update = () => setViewport({ width: window.innerWidth, height: window.innerHeight });
    window.addEventListener('resize', update);
    window.addEventListener('orientationchange', update);
    return () => { window.removeEventListener('resize', update); window.removeEventListener('orientationchange', update); };
  }, []);
  useEffect(() => {
    const flush = () => { if (changed.current) saveBrowserPaneLayout(latest.current); };
    window.addEventListener('pagehide', flush);
    return () => { window.removeEventListener('pagehide', flush); flush(); };
  }, []);
  useEffect(() => {
    if (!changed.current) return;
    const timer = window.setTimeout(() => saveBrowserPaneLayout(latest.current), 120);
    return () => window.clearTimeout(timer);
  }, [layout]);

  const activate = useCallback(() => window.dispatchEvent(new CustomEvent<string>(ACTIVE_PANE_EVENT, { detail: id })), [id]);
  useEffect(() => {
    const focus = (event: Event) => setActive((event as CustomEvent<unknown>).detail === id);
    window.addEventListener(ACTIVE_PANE_EVENT, focus);
    activate();
    return () => window.removeEventListener(ACTIVE_PANE_EVENT, focus);
  }, [activate, id]);

  const change = (update: (previous: BrowserPaneLayout) => BrowserPaneLayout) => {
    changed.current = true;
    setLayout(update);
  };
  const applyRect = (next: BrowserPaneRect) => {
    const fitted = fitBrowserPaneRect(next, viewport);
    change(previous => previous.mode === 'floating' ? { ...previous, floating: fitted } : { ...previous, dockWidth: fitted.width });
  };
  const toggleDock = () => {
    drag.current = null;
    change(previous => previous.mode === 'floating' ? { ...previous, mode: 'docked' } : { ...previous, mode: 'floating' });
  };
  const pointerDown = (event: PointerEvent<HTMLElement>, kind: DragKind) => {
    if (expanded || narrow || event.button !== 0 || event.isPrimary === false || kind === 'move' && layout.mode !== 'floating') return;
    event.preventDefault();
    event.currentTarget.focus();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    drag.current = { kind, pointer: event.pointerId, x: event.clientX, y: event.clientY, rect, mode: layout.mode };
  };
  const pointerMove = (event: PointerEvent<HTMLElement>) => {
    const current = drag.current;
    if (event.buttons === 0) { drag.current = null; return; }
    if (!current || current.pointer !== event.pointerId || current.mode !== layout.mode || expanded || narrow) return;
    const x = event.clientX - current.x, y = event.clientY - current.y;
    if (current.kind === 'move') applyRect({ ...current.rect, left: current.rect.left + x, top: current.rect.top + y });
    else applyRect({ ...current.rect, width: current.rect.width + (layout.mode === 'docked' ? -x : x),
      height: current.rect.height + (layout.mode === 'floating' ? y : 0) });
  };
  const pointerEnd = (event: PointerEvent<HTMLElement>) => {
    if (drag.current?.pointer !== event.pointerId) return;
    drag.current = null;
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
  };
  const keyDown = (event: KeyboardEvent<HTMLElement>, kind: DragKind) => {
    if (expanded || narrow || event.altKey || event.ctrlKey || event.metaKey || !['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) return;
    if (layout.mode === 'docked' && ['ArrowUp', 'ArrowDown'].includes(event.key)) return;
    event.preventDefault();
    const step = event.shiftKey ? 50 : 10;
    const x = event.key === 'ArrowRight' ? step : event.key === 'ArrowLeft' ? -step : 0;
    const y = event.key === 'ArrowDown' ? step : event.key === 'ArrowUp' ? -step : 0;
    if (kind === 'move') applyRect({ ...rect, left: rect.left + x, top: rect.top + y });
    else applyRect({ ...rect, width: rect.width + (layout.mode === 'docked' ? -x : x), height: rect.height + y });
  };
  const handles = (kind: DragKind) => ({ onPointerDown: (event: PointerEvent<HTMLElement>) => pointerDown(event, kind),
    onPointerMove: pointerMove, onPointerUp: pointerEnd, onPointerCancel: pointerEnd,
    onLostPointerCapture: () => { drag.current = null; }, onKeyDown: (event: KeyboardEvent<HTMLElement>) => keyDown(event, kind) });
  const style: CSSProperties = narrow || expanded ? {} : layout.mode === 'docked' ? { width: rect.width }
    : { left: rect.left, top: rect.top, width: rect.width, height: rect.height, right: 'auto', bottom: 'auto' };
  const resizeStyle: CSSProperties = layout.mode === 'docked'
    ? { left: rect.left - 3, top: rect.top, bottom: 8, width: 8 }
    : { left: rect.left + rect.width - 19, top: rect.top + rect.height - 19, width: 18, height: 18 };
  return { id, layout, rect, narrow, style, resizeStyle, active, activate, toggleDock, move: handles('move'), resize: handles('resize') };
}
