export const BROWSER_PANE_LAYOUT_KEY = 'webmux-browser-pane-layout-v1';
export const BROWSER_PANE_MARGIN = 8;
export const BROWSER_PANE_TOP = 64;
export const BROWSER_PANE_MOBILE_WIDTH = 650;
export type BrowserPaneRect = { left: number; top: number; width: number; height: number };
export type BrowserPaneLayout = { version: 1; mode: 'docked' | 'floating'; dockWidth: number | null; floating: BrowserPaneRect | null };
export type BrowserPaneViewport = { width: number; height: number };

export function defaultBrowserPaneLayout(): BrowserPaneLayout {
  return { version: 1, mode: 'docked', dockWidth: null, floating: null };
}

const number = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value);
const clamp = (value: number, low: number, high: number) => Math.min(Math.max(value, low), Math.max(low, high));

export function parseBrowserPaneLayout(raw: string | null): BrowserPaneLayout {
  const fallback = defaultBrowserPaneLayout();
  if (!raw || raw.length > 1024) return fallback;
  try {
    const value: unknown = JSON.parse(raw);
    if (!value || typeof value !== 'object' || !('version' in value) || value.version !== 1) return fallback;
    const known = value as Record<string, unknown>;
    const floating = known.floating as Record<string, unknown> | null;
    const validRect = floating && typeof floating === 'object' &&
      number(floating.left) && number(floating.top) && number(floating.width) && number(floating.height) &&
      Math.abs(floating.left) <= 100_000 && Math.abs(floating.top) <= 100_000 &&
      floating.width > 0 && floating.width <= 2560 && floating.height > 0 && floating.height <= 1600;
    return {
      version: 1,
      mode: known.mode === 'floating' ? 'floating' : 'docked',
      dockWidth: number(known.dockWidth) && known.dockWidth > 0 && known.dockWidth <= 2560 ? known.dockWidth : null,
      floating: validRect ? { left: floating.left as number, top: floating.top as number, width: floating.width as number, height: floating.height as number } : null,
    };
  } catch { return fallback; }
}

export function readBrowserPaneLayout(): BrowserPaneLayout {
  try { return parseBrowserPaneLayout(localStorage.getItem(BROWSER_PANE_LAYOUT_KEY)); }
  catch { return defaultBrowserPaneLayout(); }
}

export function saveBrowserPaneLayout(layout: BrowserPaneLayout): void {
  // Serialize only geometry and mode, never frame/session/profile data.
  const value: BrowserPaneLayout = { version: 1, mode: layout.mode, dockWidth: layout.dockWidth,
    floating: layout.floating && { left: layout.floating.left, top: layout.floating.top, width: layout.floating.width, height: layout.floating.height } };
  try { localStorage.setItem(BROWSER_PANE_LAYOUT_KEY, JSON.stringify(value)); } catch { /* Layout remains usable without storage. */ }
}

export function fitBrowserPaneRect(rect: BrowserPaneRect, viewport: BrowserPaneViewport): BrowserPaneRect {
  const maxWidth = Math.max(1, viewport.width - BROWSER_PANE_MARGIN * 2);
  const maxHeight = Math.max(1, viewport.height - BROWSER_PANE_TOP - BROWSER_PANE_MARGIN);
  const width = Math.round(clamp(rect.width, Math.min(340, maxWidth), Math.min(2560, maxWidth)));
  const height = Math.round(clamp(rect.height, Math.min(360, maxHeight), Math.min(1600, maxHeight)));
  return { width, height,
    left: Math.round(clamp(rect.left, BROWSER_PANE_MARGIN, viewport.width - BROWSER_PANE_MARGIN - width)),
    top: Math.round(clamp(rect.top, BROWSER_PANE_TOP, viewport.height - BROWSER_PANE_MARGIN - height)) };
}

export function browserPaneRect(layout: BrowserPaneLayout, viewport: BrowserPaneViewport): BrowserPaneRect {
  const width = layout.dockWidth ?? Math.min(880, viewport.width * 0.65);
  if (layout.mode === 'floating') {
    const initial = layout.floating ?? { width, height: Math.min(720, viewport.height - BROWSER_PANE_TOP - BROWSER_PANE_MARGIN),
      left: (viewport.width - width) / 2, top: BROWSER_PANE_TOP + 24 };
    return fitBrowserPaneRect(initial, viewport);
  }
  const fitted = fitBrowserPaneRect({ left: 0, top: BROWSER_PANE_TOP, width, height: viewport.height - BROWSER_PANE_TOP - BROWSER_PANE_MARGIN }, viewport);
  return { ...fitted, left: viewport.width - BROWSER_PANE_MARGIN - fitted.width };
}
