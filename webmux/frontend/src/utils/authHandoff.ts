export interface HandoffLink { url: string; origin: string; loopback: boolean; relay: boolean; device: boolean }

/** Classification is guidance, never a claim that an arbitrary link is trusted. */
export function handoffLink(raw: string): HandoffLink | null {
  if (raw.length > 16384 || /[\r\n\t]/.test(raw)) return null;
  try {
    const url = new URL(raw);
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) return null;
    const local = (u: URL) => ['localhost', '127.0.0.1', '[::1]'].includes(u.hostname);
    const redirect = url.searchParams.get('redirect_uri');
    let loopback = local(url), relay = false;
    if (redirect) {
      try {
        const target = new URL(redirect);
        loopback ||= local(target);
        relay = local(target) && target.protocol === 'http:' && Number(target.port) >= 1024 &&
          !target.username && !target.password && !target.search && !target.hash && !url.hash &&
          url.searchParams.getAll('redirect_uri').length === 1 && url.searchParams.getAll('state').length === 1 && !!url.searchParams.get('state');
      } catch { /* Unknown redirects keep the generic guidance. */ }
    }
    const device = url.protocol === 'https:' && !redirect && (
      (url.hostname === 'github.com' && url.pathname === '/login/device') ||
      (url.hostname === 'auth.openai.com' && url.pathname === '/codex/device') ||
      (url.hostname === 'microsoft.com' && url.pathname === '/devicelogin') ||
      (url.hostname === 'www.microsoft.com' && url.pathname === '/devicelogin')
    );
    return { url: raw, origin: url.origin, loopback, relay, device };
  } catch { return null; }
}
