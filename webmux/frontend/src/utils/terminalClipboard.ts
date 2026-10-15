import type { Terminal } from '@xterm/xterm';

export const MAX_TERMINAL_PASTE_BYTES = 128 * 1024;
export const MAX_TERMINAL_FILE_BYTES = 10 * 1024 * 1024;

/** Desktop shortcuts must reach browser copy/paste events, rather than the PTY. */
export function terminalKeyHandler(event: KeyboardEvent, selection: boolean, mac = /Mac|iPhone|iPad/.test(navigator.platform)): boolean {
  const key = event.key.toLowerCase();
  if (event.altKey) return true;
  if (event.metaKey && (key === 'c' || key === 'v')) return false;
  if (event.ctrlKey && !event.metaKey) {
    if (event.shiftKey && (key === 'c' || key === 'v')) return false;
    if (!mac && (key === 'v' || (key === 'c' && selection))) return false;
  }
  return true;
}

export function clipboardFiles(data: DataTransfer | null): File[] {
  if (!data) return [];
  const files = Array.from(data.files ?? []);
  if (files.length) return files;
  return Array.from(data.items ?? []).filter(item => item.kind === 'file').map(item => item.getAsFile()).filter((file): file is File => file !== null);
}

export function quoteTerminalPath(path: string): string {
  if (!path || Array.from(path).some(char => char.charCodeAt(0) < 32 || char.charCodeAt(0) === 127)) throw new Error('The host returned an invalid file path.');
  return `'${path.replace(/'/g, "'\\''")}'`;
}

/** Capture before xterm's textarea handler so image paste cannot disappear. */
export function installTerminalPaste(element: HTMLElement, term: Terminal, onFiles: (files: File[]) => void, connected: () => boolean, onError: (message: string) => void): () => void {
  const paste = (event: ClipboardEvent) => {
    if (!event.clipboardData) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    if (!connected()) { onError('Reconnect the terminal before pasting.'); return; }
    const files = clipboardFiles(event.clipboardData);
    if (files.length) { onFiles(files); return; }
    const text = event.clipboardData.getData('text/plain');
    if (new TextEncoder().encode(text).length > MAX_TERMINAL_PASTE_BYTES) { onError('Paste at most 128 KiB of text at a time.'); return; }
    term.paste(text);
  };
  element.addEventListener('paste', paste, true);
  return () => element.removeEventListener('paste', paste, true);
}

/** User-initiated copy still works on HTTP when the Async Clipboard API does not. */
export async function copyDesktopText(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try { await navigator.clipboard.writeText(text); return true; } catch { /* Try the browser copy command. */ }
  }
  const active = document.activeElement as HTMLElement | null;
  const input = document.createElement('textarea');
  input.value = text; input.style.position = 'fixed'; input.style.opacity = '0';
  document.body.append(input); input.select();
  try { return typeof document.execCommand === 'function' && document.execCommand('copy'); }
  finally { input.remove(); active?.focus(); }
}
