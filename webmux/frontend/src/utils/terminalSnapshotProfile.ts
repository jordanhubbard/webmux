import type { IDisposable, Terminal } from '@xterm/xterm';

export const XTERM_SNAPSHOT_PROFILE = 'webmux-xterm-state-v1';
export const XTERM_UNDERLINE_SNAPSHOT_PROFILE = 'webmux-xterm-state-v2';
const V1_PREFIX = 'webmux.replay.v1;';
const V2_PREFIX = 'webmux.replay.v2;';
const UNDERLINE_BIT = 0x10000000;
const TOKEN = /^[a-f0-9]{32}$/;
const BASE64 = /^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/;
const installed = new WeakSet<object>();
type Parser = { precedingJoinState: number };
type Pen = { fg: number; bg: number; clone(): Pen; isUnderline(): number; hasExtendedAttrs(): number };
type Core = { _bufferService: { buffer: { x: number }; buffers: { normal: { savedX: number }; alt: { savedX: number } } }; _inputHandler: { _curAttrData: Pen; _parser: Parser; _unicodeService: {
  charProperties(codepoint: number, previous: number): number;
} } };

export interface TerminalSnapshotProfile extends IDisposable {
  // Configure only from authenticated transport metadata, never terminal output.
  // Missing tokens keep the extension disabled for ordinary/older streams.
  setToken(token?: string, profile?: string): void;
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
  let version = XTERM_SNAPSHOT_PROFILE;
  let disposed = false;
  const get = () => state;
  const set = (value: number) => {
    state = pending !== undefined && value === 0 ? pending : value;
    pending = undefined;
  };
  Object.defineProperty(parser, 'precedingJoinState', { configurable: true, enumerable: descriptor.enumerable, get, set });
  const packet = term.parser.registerOscHandler(7777, data => {
    const v2 = data.startsWith(V2_PREFIX);
    if (!v2 && !data.startsWith(V1_PREFIX)) return false;
    const prefix = v2 ? V2_PREFIX : V1_PREFIX;
    if (disposed || !token || version !== (v2 ? XTERM_UNDERLINE_SNAPSHOT_PROFILE : XTERM_SNAPSHOT_PROFILE) || data.length > prefix.length + 33 + (v2 ? 256 : 88)) return true;
    const body = data.slice(prefix.length);
    if (body.slice(0, 32) !== token || body[32] !== ';') return true;
    const encoded = body.slice(33);
    if (!encoded || !BASE64.test(encoded)) return true;
    try {
      const bytes = Uint8Array.from(atob(encoded), c => c.charCodeAt(0));
      if (!bytes.length || bytes.length > (v2 ? 192 : 64)) return true;
      let text = new TextDecoder('utf-8', { fatal: true }).decode(bytes);
      let flags: number | undefined;
      let columns: { x?: number; n?: number; a?: number } | undefined;
      if (v2) {
        const value: unknown = JSON.parse(text);
        if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).some(key => !['u', 'r', 'x', 'n', 'a'].includes(key)) || !('u' in value) || !('r' in value)
          || !Number.isInteger(value.u) || (value.u as number) < 1 || (value.u as number) > 4 || typeof value.r !== 'string') return true;
        for (const key of ['x', 'n', 'a'] as const) {
          if (key in value && (!Number.isInteger(value[key as keyof typeof value]) || (value[key as keyof typeof value] as number) < 0 || (value[key as keyof typeof value] as number) > term.cols)) return true;
        }
        flags = value.u as number;
        columns = value as { x?: number; n?: number; a?: number };
        text = value.r;
        if (new TextEncoder().encode(text).length > 64) return true;
        // JSON can contain escaped lone surrogates even with valid UTF-8 bytes.
        // Refuse them rather than letting the renderer substitute a character.
        if (Array.from(text).some(c => { const codepoint = c.codePointAt(0)!; return codepoint >= 0xd800 && codepoint <= 0xdfff; })) return true;
      }
      let next = 0;
      for (const character of text) {
        const codepoint = character.codePointAt(0)!;
        if (codepoint < 0x20 || codepoint >= 0x7f && codepoint <= 0x9f) return true;
        next = unicode.charProperties(codepoint, next);
      }
      if (!Number.isSafeInteger(next) || next < 0 || next > 0x7fffffff || (!v2 && next === 0) || (text.length > 0 && next === 0)) return true;
      if (flags !== undefined) {
        // Alter only the two current flag bits. In particular, do not replace
        // the shared extended object, saved attributes, charset or protection.
        // Optional columns repair scratch RC clamping without repainting cells.
        const pen = input._curAttrData;
        pen.fg = (pen.fg & ~UNDERLINE_BIT) | ((flags - 1) & 1 ? UNDERLINE_BIT : 0);
        pen.bg = (pen.bg & ~UNDERLINE_BIT) | ((flags - 1) & 2 ? UNDERLINE_BIT : 0);
        if (columns?.x !== undefined) core._bufferService.buffer.x = columns.x;
        if (columns?.n !== undefined) core._bufferService.buffers.normal.savedX = columns.n;
        if (columns?.a !== undefined) core._bufferService.buffers.alt.savedX = columns.a;
      }
      // A token restores one snapshot only; later terminal output cannot
      // reuse a previously accepted packet. A new attach supplies new metadata.
      token = undefined;
      pending = next;
    } catch { /* A malformed packet is an ignored OSC, with no I/O effects. */ }
    return true;
  });
  return {
    setToken(value, profile = XTERM_SNAPSHOT_PROFILE) {
      if (disposed) throw new Error('The terminal snapshot profile is disposed');
      if (value !== undefined && !TOKEN.test(value)) throw new Error('Invalid terminal snapshot token');
      if (profile !== XTERM_SNAPSHOT_PROFILE && profile !== XTERM_UNDERLINE_SNAPSHOT_PROFILE) throw new Error('Unknown terminal snapshot profile');
      if (profile === XTERM_UNDERLINE_SNAPSHOT_PROFILE) {
        // Qualify the pinned attribute bit layout on a detached value. Re-read
        // the live pen when accepting a packet because RIS replaces that pen.
        const pen = input._curAttrData;
        if (!pen || !Number.isInteger(pen.fg) || !Number.isInteger(pen.bg) || typeof pen.clone !== 'function' || typeof pen.isUnderline !== 'function' || typeof pen.hasExtendedAttrs !== 'function') throw new Error('This renderer does not support independent underline flags');
        const buffers = core._bufferService;
        if (!Number.isInteger(buffers?.buffer?.x) || !Number.isInteger(buffers?.buffers?.normal?.savedX) || !Number.isInteger(buffers?.buffers?.alt?.savedX)) throw new Error('Unknown terminal cursor layout');
        const probe = pen.clone();
        if (probe === pen) throw new Error('Unknown terminal attribute clone behavior');
        probe.fg &= ~UNDERLINE_BIT; probe.bg &= ~UNDERLINE_BIT;
        if (probe.isUnderline() || probe.hasExtendedAttrs()) throw new Error('Unknown terminal attribute layout');
        probe.fg |= UNDERLINE_BIT; probe.bg |= UNDERLINE_BIT;
        if (!probe.isUnderline() || !probe.hasExtendedAttrs()) throw new Error('Unknown terminal attribute layout');
      }
      token = value; version = profile; pending = undefined;
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
