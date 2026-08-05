import type { AppFontFaceConfig } from '../types';

export const DEFAULT_TERMINAL_FONT_FAMILY = 'ui-monospace, "SFMono-Regular", Monaco, Menlo, Consolas, "Liberation Mono", "DejaVu Sans Mono", monospace';
export const TERMINAL_FONT_CSS_VARIABLE = '--webmux-mono-font';
export const TERMINAL_FONTS_LOADED_EVENT = 'webmux:terminal-fonts-loaded';

const installedFontFaces = new Set<string>();

export function normalizeTerminalFontFamily(fontFamily: string | undefined | null): string {
  const trimmed = fontFamily?.trim();
  return trimmed || DEFAULT_TERMINAL_FONT_FAMILY;
}

function authHeaders(): HeadersInit {
  const token = localStorage.getItem('webmux_token');
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function fontFaceKey(face: AppFontFaceConfig): string {
  return [
    face.family,
    face.url,
    face.source,
    face.weight ?? '',
    face.style ?? '',
    face.display ?? '',
  ].join('\0');
}

export async function installConfiguredFontFaces(fontFaces: AppFontFaceConfig[] | undefined | null): Promise<void> {
  if (
    typeof document === 'undefined' ||
    !('fonts' in document) ||
    typeof FontFace === 'undefined' ||
    !Array.isArray(fontFaces)
  ) {
    return;
  }

  let loadedAny = false;
  await Promise.all(fontFaces.map(async face => {
    if (!face.url) return;
    const key = fontFaceKey(face);
    if (installedFontFaces.has(key)) return;

    try {
      const response = await fetch(face.url, { headers: authHeaders() });
      if (!response.ok) return;
      const fontData = await response.arrayBuffer();
      const fontFace = new FontFace(face.family, fontData, {
        weight: String(face.weight ?? '400'),
        style: face.style ?? 'normal',
        display: face.display ?? 'swap',
      });
      const loaded = await fontFace.load();
      document.fonts.add(loaded);
      installedFontFaces.add(key);
      loadedAny = true;
    } catch {
      // A bad custom font should not prevent the configured fallback stack from rendering.
    }
  }));

  if (loadedAny) {
    window.dispatchEvent(new Event(TERMINAL_FONTS_LOADED_EVENT));
  }
}

export async function loadTerminalFontFamily(fontFamily: string | undefined | null, fontSize: number): Promise<void> {
  if (typeof document === 'undefined' || !('fonts' in document)) return;

  try {
    const normalizedFontFamily = normalizeTerminalFontFamily(fontFamily);
    await document.fonts.load(`${fontSize}px ${normalizedFontFamily}`);
    await document.fonts.ready;
  } catch {
    // Invalid or unavailable font stacks should still fall through to browser fallback fonts.
  }
}
