import type { IDisposable, Terminal } from '@xterm/xterm';

const ESC = String.fromCharCode(0x1b);
const ESC_PATTERN = '\\x1b';
const BEL_PATTERN = '\\x07';
const TERMINAL_IDENTITY_RESPONSE_BODY_PATTERN = /^(?:[?>]?[0-9;]*)$/;
const CURSOR_POSITION_REPORT_PATTERN = new RegExp(`^${ESC_PATTERN}\\[\\d+;\\d+R$`);
const DEVICE_STATUS_REPORT_PATTERN = new RegExp(`^${ESC_PATTERN}\\[\\d+n$`);
const OSC_RESPONSE_PATTERN = new RegExp(
  `^${ESC_PATTERN}\\][\\s\\S]*(?:${BEL_PATTERN}|${ESC_PATTERN}\\\\)$`,
);

/** OSC identifiers used by termenv/gh for color and palette queries. */
const OSC_COLOR_QUERY_IDS = [4, 10, 11, 12, 104, 110, 111] as const;

export function isTerminalIdentityResponse(data: string): boolean {
  if (!data.startsWith(`${ESC}[`) || !data.endsWith('c')) return false;
  return TERMINAL_IDENTITY_RESPONSE_BODY_PATTERN.test(data.slice(2, -1));
}

/**
 * Returns true when xterm.js auto-generated terminal query responses should not
 * be forwarded to the remote PTY. Remote programs (e.g. gh via termenv) send
 * OSC/CPR queries and break if those replies arrive on stdin later.
 */
export function shouldSuppressTerminalInput(data: string): boolean {
  if (!data.includes(ESC)) return false;
  if (isTerminalIdentityResponse(data)) return true;
  if (CURSOR_POSITION_REPORT_PATTERN.test(data)) return true;
  if (DEVICE_STATUS_REPORT_PATTERN.test(data)) return true;
  if (OSC_RESPONSE_PATTERN.test(data)) return true;
  return false;
}

/**
 * Swallow terminal capability queries in the parser so xterm.js does not emit
 * OSC/CSI replies back through onData to the remote session.
 */
export function installTerminalQuerySuppressors(term: Terminal): IDisposable {
  const disposables: IDisposable[] = [
    term.parser.registerCsiHandler({ final: 'c' }, () => true),
    term.parser.registerCsiHandler({ prefix: '>', final: 'c' }, () => true),
    term.parser.registerCsiHandler({ final: 'n' }, (params) => {
      const ps = params[0];
      return ps === 5 || ps === 6;
    }),
    ...OSC_COLOR_QUERY_IDS.map((ident) =>
      term.parser.registerOscHandler(ident, (payload) => payload.includes('?')),
    ),
  ];

  return {
    dispose: () => {
      disposables.forEach((disposable) => disposable.dispose());
    },
  };
}
