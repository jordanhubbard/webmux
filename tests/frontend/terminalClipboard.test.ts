import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Terminal } from '@xterm/xterm';
import { clipboardFiles, installTerminalPaste, quoteTerminalPath, terminalKeyHandler } from '@frontend/utils/terminalClipboard';

afterEach(() => vi.restoreAllMocks());
describe('desktop terminal input', () => {
 it('leaves desktop shortcuts to the browser and preserves terminal interrupts', () => {
  expect(terminalKeyHandler(new KeyboardEvent('keydown', { key: 'v', ctrlKey: true }), false, false)).toBe(false);
  expect(terminalKeyHandler(new KeyboardEvent('keydown', { key: 'c', ctrlKey: true }), true, false)).toBe(false);
  expect(terminalKeyHandler(new KeyboardEvent('keydown', { key: 'c', ctrlKey: true }), false, false)).toBe(true);
  expect(terminalKeyHandler(new KeyboardEvent('keydown', { key: 'v', metaKey: true }), false, true)).toBe(false);
  expect(terminalKeyHandler(new KeyboardEvent('keydown', { key: 'C', ctrlKey: true, shiftKey: true }), false, false)).toBe(false);
 });
 it('quotes filenames without injecting shell syntax', () => {
  expect(quoteTerminalPath("/tmp/a'b $(touch unsafe).png")).toBe("'/tmp/a'\\''b $(touch unsafe).png'");
  expect(() => quoteTerminalPath('/tmp/file\nescape')).toThrow();
 });
 it('extracts pasted screenshots from clipboard items when files is empty', () => {
  const file = new File(['image'], 'screenshot.png', { type: 'image/png' });
  const data = { files: [], items: [{ kind: 'file', getAsFile: () => file }] } as unknown as DataTransfer;
  expect(clipboardFiles(data)).toEqual([file]);
 });
 it('routes image paste once before xterm can discard it', () => {
  const element = document.createElement('div'); const textarea = document.createElement('textarea'); element.append(textarea);
  const file = new File(['image'], 'screenshot.png', { type: 'image/png' });
  const term = { paste: vi.fn() }; const files = vi.fn(); const error = vi.fn(); const xtermPaste = vi.fn();
  textarea.addEventListener('paste', xtermPaste);
  const remove = installTerminalPaste(element, term as unknown as Terminal, files, () => true, error);
  const event = new Event('paste', { bubbles: true, cancelable: true });
  Object.defineProperty(event, 'clipboardData', { value: { files: [file], getData: () => 'ignored duplicate text' } });
  textarea.dispatchEvent(event);
  expect(files).toHaveBeenCalledExactlyOnceWith([file]); expect(term.paste).not.toHaveBeenCalled(); expect(xtermPaste).not.toHaveBeenCalled(); expect(event.defaultPrevented).toBe(true);
  remove();
 });
 it('uses xterm paste framing for text and refuses disconnected or oversized pastes', () => {
  const element = document.createElement('div'); const term = { paste: vi.fn() }; const files = vi.fn(); const error = vi.fn(); let connected = true;
  const remove = installTerminalPaste(element, term as unknown as Terminal, files, () => connected, error);
  const paste = (text: string) => {
   const event = new Event('paste', { bubbles: true, cancelable: true });
   Object.defineProperty(event, 'clipboardData', { value: { files: [], items: [], getData: () => text } }); element.dispatchEvent(event);
  };
  paste('line 1\nline 2'); expect(term.paste).toHaveBeenCalledExactlyOnceWith('line 1\nline 2');
  paste('x'.repeat(128 * 1024 + 1)); expect(error).toHaveBeenLastCalledWith('Paste at most 128 KiB of text at a time.');
  connected = false; paste('blocked'); expect(term.paste).toHaveBeenCalledTimes(1); expect(error).toHaveBeenLastCalledWith('Reconnect the terminal before pasting.');
  remove();
 });
});
