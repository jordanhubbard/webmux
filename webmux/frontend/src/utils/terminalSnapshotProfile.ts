import type { IDisposable, Terminal } from '@xterm/xterm';

export const XTERM_SNAPSHOT_PROFILE = 'webmux-xterm-state-v1';
const PREFIX = 'webmux.replay.v1;';
const TOKEN = /^[a-f0-9]{32}$/;
const BASE64 = /^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/;
const installed = new WeakSet<object>();
type Parser = { precedingJoinState: number };
type Core = { _inputHandler: { _parser: Parser; _unicodeService: {
  charProperties(codepoint: number, previous: number): number;
} } };

export interface TerminalSnapshotProfile extends IDisposable {
  // Configure only from authenticated transport metadata, never terminal output.
  // Missing tokens keep the extension disabled for ordinary/older streams.
  setToken(token?: string): void;
}

/** Restore parser state only for an explicitly negotiated snapshot token.
 * The pinned renderer clears precedingJoinState immediately after an OSC
 * handler returns. Override exactly that one reset, then preserve normal
 * parser behavior. Actual xterm comparisons qualify this private adapter.
 */
export function installTerminalSnapshotProfile(term: Terminal): TerminalSnapshotProfile {
  const core = (term as unknown as { _core: Core })._core;
  const input = core?._inputHandler;
  const parser = input?._parser;
  const unicode = input?._unicodeService;
  const descriptor = parser && Object.getOwnPropertyDescriptor(parser, 'precedingJoinState');
  if (!parser || !unicode || typeof unicode.charProperties !== 'function' || !descriptor?.configurable || !descriptor.writable || typeof descriptor.value !== 'number' || installed.has(parser)) {
    throw new Error('This renderer does not support the WebMux snapshot profile');
  }
  installed.add(parser);
  let state: number = descriptor.value;
  let pending: number | undefined;
  let token: string | undefined;
  let disposed = false;
  const get = () => state;
  const set = (value: number) => {
    state = pending !== undefined && value === 0 ? pending : value;
    pending = undefined;
  };
  Object.defineProperty(parser, 'precedingJoinState', { configurable: true, enumerable: descriptor.enumerable, get, set });
  const packet = term.parser.registerOscHandler(7777, data => {
    if (!data.startsWith(PREFIX)) return false;
    if (disposed || !token || data.length > PREFIX.length + 33 + 88) return true;
    const body = data.slice(PREFIX.length);
    if (body.slice(0, 32) !== token || body[32] !== ';') return true;
    const encoded = body.slice(33);
    if (!encoded || encoded.length > 88 || !BASE64.test(encoded)) return true;
    try {
      const bytes = Uint8Array.from(atob(encoded), c => c.charCodeAt(0));
      if (!bytes.length || bytes.length > 64) return true;
      const text = new TextDecoder('utf-8', { fatal: true }).decode(bytes);
      let next = 0;
      for (const character of text) {
        const codepoint = character.codePointAt(0)!;
        if (codepoint < 0x20 || codepoint >= 0x7f && codepoint <= 0x9f) return true;
        next = unicode.charProperties(codepoint, next);
      }
      if (!Number.isSafeInteger(next) || next <= 0 || next > 0x7fffffff) return true;
      // A token restores one snapshot only; later terminal output cannot
      // reuse a previously accepted packet. A new attach supplies new metadata.
      token = undefined;
      pending = next;
    } catch { /* A malformed packet is an ignored OSC, with no I/O effects. */ }
    return true;
  });
  return {
    setToken(value) {
      if (disposed) throw new Error('The terminal snapshot profile is disposed');
      if (value !== undefined && !TOKEN.test(value)) throw new Error('Invalid terminal snapshot token');
      token = value; pending = undefined;
    },
    dispose() {
      if (disposed) return;
      disposed = true; token = undefined; pending = undefined;
      packet.dispose();
      if (Object.getOwnPropertyDescriptor(parser, 'precedingJoinState')?.get === get) {
        Object.defineProperty(parser, 'precedingJoinState', { ...descriptor, value: state });
      }
      installed.delete(parser);
    },
  };
}
