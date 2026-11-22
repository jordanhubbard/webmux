// Wheel ownership is decided before terminal/application mouse handlers run.
// Ordinary browser scroll chaining happens after bubbling, so stop it there.
export function installGridWheel(root: HTMLElement, selectedSession: () => string | null): () => void {
  const forwarded = new WeakSet<Event>();
  const nativeControl = (event: WheelEvent) => event.target instanceof Element
    && !event.target.closest('[data-terminal-session]')
    && Boolean(event.target.closest('input, textarea, select, button'));
  const otherModifier = (event: WheelEvent) => event.ctrlKey || event.metaKey || event.altKey;
  const capture = (event: WheelEvent) => {
    if (forwarded.has(event) || otherModifier(event)) return;
    if (event.shiftKey) {
      event.preventDefault();
      event.stopImmediatePropagation();
      const x = event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? root.clientWidth : 1;
      const y = event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? root.clientHeight : 1;
      root.scrollLeft += event.deltaX * x;
      root.scrollTop += event.deltaY * y;
      return;
    }
    if (nativeControl(event) || (event.target instanceof Element && event.target.closest('[data-wheel-native]'))) return;
    const id = selectedSession();
    const windows = root.querySelectorAll<HTMLElement>('[data-terminal-session]');
    const selected = Array.from(windows).find(element => element.dataset.terminalSession === id);
    const surface = selected?.querySelector<HTMLElement>('.xterm-screen');
    if (!surface || !(event.target instanceof Node) || surface.contains(event.target)) return;
    // Keep the selected session as the gesture owner even over another tile or
    // grid gap. Native xterm handlers still own scrollback and app mouse input.
    event.preventDefault();
    event.stopImmediatePropagation();
    const rect = surface.getBoundingClientRect();
    const routed = new WheelEvent('wheel', {
      bubbles: true, cancelable: true, composed: true, view: event.view,
      deltaX: event.deltaX, deltaY: event.deltaY, deltaZ: event.deltaZ, deltaMode: event.deltaMode,
      clientX: rect.left + rect.width / 2, clientY: rect.top + rect.height / 2,
    });
    // Chromium synthesizes legacy wheelDelta fields from constructor deltas
    // with different semantics than real input. Preserve the original fields
    // for consumers that still normalize through those browser properties.
    for (const key of ['wheelDelta', 'wheelDeltaX', 'wheelDeltaY']) {
      const value: unknown = Reflect.get(event, key);
      if (typeof value === 'number') Object.defineProperty(routed, key, { value });
    }
    forwarded.add(routed);
    surface.dispatchEvent(routed);
  };
  const contain = (event: WheelEvent) => {
    if (!event.shiftKey && !otherModifier(event) && !nativeControl(event)) event.preventDefault();
  };
  root.addEventListener('wheel', capture, { capture: true, passive: false });
  root.addEventListener('wheel', contain, { passive: false });
  return () => {
    root.removeEventListener('wheel', capture, true);
    root.removeEventListener('wheel', contain);
  };
}
