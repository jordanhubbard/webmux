import { useEffect, useState } from 'react';
import { api } from '../utils/api';
import { handoffLink } from '../utils/authHandoff';
import type { Session } from '../types';
import './AuthHandoff.css';

/** Session-scoped and nonmodal: the user can still read the CLI's device code. */
export function AuthHandoff({ session, url, onDismiss, onCompanion }: {
  session: Session; url: string; onDismiss: () => void; onCompanion: (url: string) => void;
}) {
  const [address, setAddress] = useState(url);
  const [ticket, setTicket] = useState('');
  const [callback, setCallback] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [delivered, setDelivered] = useState(false);
  const [expired, setExpired] = useState(false);
  const link = handoffLink(address);
  useEffect(() => {
    const timer = setTimeout(() => { setTicket(''); setCallback(''); setExpired(true); }, 5 * 60_000);
    return () => clearTimeout(timer);
  }, []);

  const prepare = async () => {
    setBusy(true); setError('');
    try { setTicket((await api.beginAuthHandoff(session.id, address)).id); }
    catch (err) { setError(err instanceof Error ? err.message : 'Cannot prepare callback relay'); }
    finally { setBusy(false); }
  };
  const deliver = async () => {
    setBusy(true); setError('');
    try {
      await api.deliverAuthHandoff(session.id, ticket, callback);
      setCallback(''); setTicket(''); setDelivered(true);
    } catch (err) { setError(err instanceof Error ? err.message : 'Callback delivery failed'); }
    finally { setBusy(false); }
  };
  const dismiss = () => {
    if (ticket) void api.endAuthHandoff(session.id, ticket).catch(() => { /* Server expiry remains authoritative. */ });
    onDismiss();
  };

  return <section className="auth-handoff" aria-label={`Sign-in for ${session.title}`} onKeyDown={e => e.stopPropagation()}>
    <header><strong>Sign-in / open link</strong><button onClick={dismiss} disabled={busy}>Dismiss sign-in</button></header>
    <p><strong>{session.title}</strong> · {session.browser_local || session.transport === 'local' ? 'WebMux server' : session.hostname}</p>
    {!url && <label>Sign-in URL<input aria-label="Sign-in URL" value={address} disabled={!!ticket || busy} onChange={e => setAddress(e.target.value)} placeholder="Paste the link printed by the CLI" /></label>}
    {expired ? <p role="status">This handoff expired. Start sign-in again in the terminal and open the new link.</p> : delivered ?
      <p role="status">Callback delivered to this terminal’s host. Check the CLI for authentication success.</p> : <>
      {link ? <>
        <p className="auth-destination">Destination: <strong>{link.origin}</strong></p>
        <p>{link.device ? 'Enter the one-time code shown in this terminal. The provider returns approval to the waiting CLI automatically.' :
          link.loopback ? 'This link uses localhost on the terminal’s host. For sign-in in your browser, prepare the callback relay below, or use the browser on the terminal host.' :
          'Use your browser for device-code or copy/paste-code sign-in. Enter the device code from this terminal; if the provider gives you a return code, paste it into this terminal when the CLI asks.'}</p>
        <p>Your browser keeps its existing sign-in and passkeys. Only approve a request you started. Return here and check the CLI for success.</p>
        {link.relay && !ticket && <button disabled={busy} onClick={() => void prepare()}>Prepare callback relay</button>}
        {(!link.loopback || ticket) && <a href={link.url} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">Continue in this browser</a>}
        {ticket && <form onSubmit={e => { e.preventDefault(); void deliver(); }}>
          <p>After signing in, the other tab may fail to load localhost. Copy its entire final address and paste it here. WebMux sends it once to this terminal’s host. Do not paste it into the shell.</p>
          <label>Final callback URL<input type="text" aria-label="Final callback URL" autoComplete="off" spellCheck={false} value={callback} onChange={e => setCallback(e.target.value)} /></label>
          <button disabled={busy || !callback} type="submit">Deliver callback</button>
        </form>}
        <button disabled={busy} onClick={() => { dismiss(); onCompanion(link.url); }}>Use browser on terminal host</button>
      </> : address && <p role="alert">Enter an HTTP or HTTPS URL without embedded credentials.</p>}
      {error && <p role="alert">{error}</p>}
    </>}
  </section>;
}
