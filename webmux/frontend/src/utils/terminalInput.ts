const ESC = String.fromCharCode(0x1b);
const TERMINAL_IDENTITY_RESPONSE_BODY_PATTERN = /^(?:[?>]?[0-9;]*)$/;

export function isTerminalIdentityResponse(data: string): boolean {
  if (!data.startsWith(`${ESC}[`) || !data.endsWith('c')) return false;
  return TERMINAL_IDENTITY_RESPONSE_BODY_PATTERN.test(data.slice(2, -1));
}
