export const DEFAULT_TERMINAL_FONT_FAMILY = 'Consolas, Menlo, "DejaVu Sans Mono", monospace';
export const TERMINAL_FONT_CSS_VARIABLE = '--webmux-mono-font';

export function normalizeTerminalFontFamily(fontFamily: string | undefined | null): string {
  const trimmed = fontFamily?.trim();
  return trimmed || DEFAULT_TERMINAL_FONT_FAMILY;
}
