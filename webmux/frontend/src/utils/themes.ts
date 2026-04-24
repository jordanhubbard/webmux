import type { NamedTheme, TerminalTheme } from '../types';

const ITERM_TO_XTERM: Record<string, keyof TerminalTheme> = {
  'Ansi 0 Color': 'black',
  'Ansi 1 Color': 'red',
  'Ansi 2 Color': 'green',
  'Ansi 3 Color': 'yellow',
  'Ansi 4 Color': 'blue',
  'Ansi 5 Color': 'magenta',
  'Ansi 6 Color': 'cyan',
  'Ansi 7 Color': 'white',
  'Ansi 8 Color': 'brightBlack',
  'Ansi 9 Color': 'brightRed',
  'Ansi 10 Color': 'brightGreen',
  'Ansi 11 Color': 'brightYellow',
  'Ansi 12 Color': 'brightBlue',
  'Ansi 13 Color': 'brightMagenta',
  'Ansi 14 Color': 'brightCyan',
  'Ansi 15 Color': 'brightWhite',
  'Background Color': 'background',
  'Foreground Color': 'foreground',
  'Cursor Color': 'cursor',
  'Cursor Text Color': 'cursorAccent',
  'Selection Color': 'selectionBackground',
};

export function parseItermColors(xml: string): TerminalTheme {
  const theme: TerminalTheme = {};
  const entryRe = /<key>([^<]+)<\/key>\s*<dict>([\s\S]*?)<\/dict>/g;
  let m: RegExpExecArray | null;
  while ((m = entryRe.exec(xml)) !== null) {
    const itermKey = m[1].trim();
    const xtermKey = ITERM_TO_XTERM[itermKey];
    if (!xtermKey) continue;
    const body = m[2];
    const readComponent = (name: string): number | null => {
      const rm = new RegExp(`<key>${name}<\\/key>\\s*<real>([^<]+)<\\/real>`).exec(body);
      return rm ? parseFloat(rm[1]) : null;
    };
    const r = readComponent('Red Component');
    const g = readComponent('Green Component');
    const b = readComponent('Blue Component');
    if (r === null || g === null || b === null) continue;
    const to255 = (f: number) => Math.max(0, Math.min(255, Math.round(f * 255)));
    theme[xtermKey] = `#${to255(r).toString(16).padStart(2, '0')}${to255(g).toString(16).padStart(2, '0')}${to255(b).toString(16).padStart(2, '0')}`;
  }
  return theme;
}

interface ThemeIndexEntry {
  name: string;
  file: string;
}

export async function loadBundledThemes(): Promise<NamedTheme[]> {
  const idxRes = await fetch('/themes/index.json', { cache: 'no-cache' });
  if (!idxRes.ok) return [];
  const idx = await idxRes.json() as { themes: ThemeIndexEntry[] };
  const results: NamedTheme[] = [];
  for (const entry of idx.themes) {
    try {
      const res = await fetch(`/themes/${encodeURIComponent(entry.file)}`, { cache: 'no-cache' });
      if (!res.ok) continue;
      const xml = await res.text();
      results.push({ name: entry.name, theme: parseItermColors(xml) });
    } catch {
      // ignore a single broken theme; keep others
    }
  }
  results.sort((a, b) => a.name.localeCompare(b.name));
  return results;
}

const GLOBAL_KEY = 'webmux_global_theme';
const OVERRIDES_KEY = 'webmux_session_themes';

export function loadGlobalTheme(): string | null {
  try {
    return localStorage.getItem(GLOBAL_KEY) || null;
  } catch {
    return null;
  }
}

export function saveGlobalTheme(name: string | null): void {
  try {
    if (name) localStorage.setItem(GLOBAL_KEY, name);
    else localStorage.removeItem(GLOBAL_KEY);
  } catch { /* quota or unavailable */ }
}

export function loadSessionThemeOverrides(): Map<string, string> {
  try {
    const raw = localStorage.getItem(OVERRIDES_KEY);
    if (!raw) return new Map();
    const obj = JSON.parse(raw) as Record<string, string>;
    return new Map(Object.entries(obj));
  } catch {
    return new Map();
  }
}

export function saveSessionThemeOverrides(map: Map<string, string>): void {
  try {
    const obj: Record<string, string> = {};
    map.forEach((v, k) => { obj[k] = v; });
    localStorage.setItem(OVERRIDES_KEY, JSON.stringify(obj));
  } catch { /* quota or unavailable */ }
}
