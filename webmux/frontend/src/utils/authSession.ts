export const AUTH_EXPIRED_EVENT = 'webmux:auth-expired';

export function getTokenExpiration(token: string | null): number | null {
  if (!token) return null;
  try {
    const payload = JSON.parse(atob(token.split('.')[1])) as { exp?: unknown };
    return typeof payload.exp === 'number' ? payload.exp * 1000 : null;
  } catch {
    return null;
  }
}

export function signalAuthExpired(): void {
  window.dispatchEvent(new Event(AUTH_EXPIRED_EVENT));
}
