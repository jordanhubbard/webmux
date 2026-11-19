import { useEffect, useRef, useState } from 'react';
import { api, type USBForwardingLease, type USBForwardingTicket } from '../utils/api';
import { Dialog } from './Dialog';
import './USBForwardDialog.css';

function leaseStatus(lease: USBForwardingLease, unavailable: boolean): string {
  if (lease.phase === 'ended' && lease.remote_port === 0) return 'Command canceled; forwarding never started';
  if (lease.phase === 'ended') return lease.release_confirmed ? 'Forwarding ended; remote listener released' : 'Forwarding ended; check remote release';
  if (lease.phase === 'failed') return lease.release_confirmed ? 'Forwarding failed; remote listener released' : 'Forwarding failed; check the local helper';
  if (lease.phase === 'expired' && lease.release_confirmed && lease.remote_port === 0) return 'Command expired; forwarding never started';
  if (lease.phase === 'uncertain' || lease.phase === 'expired') return 'Remote release not confirmed; check the local helper';
  if (unavailable || lease.stale) return 'Status unavailable; the local lease deadline still applies';
  if (lease.phase === 'stopping') return 'Stop requested; waiting for remote release';
  if (lease.phase === 'pending') return 'Waiting for the local command';
  if (lease.phase === 'starting') return 'Starting forwarding';
  return `Transport ready on 127.0.0.1:${lease.remote_port}; check device attachment in the native importer`;
}
function dashboardCommand(ticket: USBForwardingTicket): string {
  const { spec } = ticket.lease;
  const bytes = new TextEncoder().encode(JSON.stringify({
    version: 1, server: window.location.origin, id: ticket.lease.id,
    pair_code: ticket.pair_code, allow_http: window.location.protocol === 'http:',
  }));
  const link = btoa(String.fromCharCode(...bytes)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  return `webmux usb-forward --host ${spec.host} --ssh-port ${spec.ssh_port} --export-port ${spec.export_port} --duration ${spec.seconds}s --dashboard-link ${link}`;
}

export function USBForwardDialog({ onClose }: { onClose: () => void }) {
  const [host, setHost] = useState('');
  const [sshPort, setSSHPort] = useState('22');
  const [exportPort, setExportPort] = useState('7575');
  const [minutes, setMinutes] = useState('30');
  const [copyStatus, setCopyStatus] = useState('');
  const [ticket, setTicket] = useState<USBForwardingTicket | null>(null);
  const [leases, setLeases] = useState<USBForwardingLease[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [unavailable, setUnavailable] = useState(false);
  const revision = useRef(0);
  const commandRef = useRef<HTMLTextAreaElement>(null);
  const statusRef = useRef<HTMLHeadingElement>(null);

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const refresh = async () => {
      const observedRevision = revision.current;
      try {
        const values = await api.getUSBForwardings(controller.signal);
        if (!controller.signal.aborted && observedRevision === revision.current) { setLeases(values); setUnavailable(false); }
      } catch {
        if (!controller.signal.aborted) setUnavailable(true);
      } finally {
        if (!controller.signal.aborted) timer = setTimeout(() => void refresh(), 2000);
      }
    };
    void refresh();
    return () => { controller.abort(); clearTimeout(timer); };
  }, []);

  async function createCommand() {
    revision.current++;
    setBusy(true); setError(''); setCopyStatus(''); setTicket(null);
    try {
      const value = await api.createUSBForwarding({
        host, ssh_port: Number(sshPort), export_port: Number(exportPort), seconds: Number(minutes) * 60,
      });
      revision.current++;
      setTicket(value);
      setLeases(previous => [value.lease, ...previous.filter(lease => lease.id !== value.lease.id)]);
      commandRef.current?.focus();
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Cannot create forwarding command'); }
    finally { setBusy(false); }
  }
  async function stopForwarding(id: string) {
    revision.current++;
    setBusy(true); setError('');
    try {
      const value = await api.stopUSBForwarding(id);
      revision.current++;
      setLeases(previous => previous.map(lease => lease.id === id ? value : lease));
      statusRef.current?.focus();
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Cannot stop forwarding'); }
    finally { setBusy(false); }
  }

  const validPort = (value: string, minimum: number) => /^\d+$/.test(value)
    && Number(value) >= minimum && Number(value) <= 65535;
  // Deliberately match the CLI's restricted destination grammar. This command
  // must be safe to paste into POSIX shells, PowerShell and cmd.exe.
  const validHost = host.length <= 253
    && /^([a-zA-Z0-9_][a-zA-Z0-9_.-]*@)?[a-zA-Z0-9_][a-zA-Z0-9_.-]*$/.test(host);
  const valid = validHost && validPort(sshPort, 1) && validPort(exportPort, 1024);
  const command = ticket ? dashboardCommand(ticket) : '';
  const commandUsable = !busy && !!ticket && leases.some(lease => lease.id === ticket.lease.id && lease.phase === 'pending');

  async function copyCommand() {
    try {
      await navigator.clipboard.writeText(command);
      setCopyStatus('Command copied. Run it on the computer hosting the USB device or simulator.');
    } catch {
      setCopyStatus('Clipboard unavailable. Select and copy the command below.');
    }
  }

  return (
    <Dialog title="USB redirection" subtitle="Experimental · native USB software required" onClose={onClose} width={620}>
      <div className="usb-forward">
        <p className="usb-forward-route"><span aria-hidden="true">🔌</span> Device or simulator computer → SSH → Computer using the device</p>
        <p>Linux, Windows and macOS peers can use different operating systems. Both ends need
          compatible USB sharing software; the receiving computer also needs the device’s driver.</p>
        <ol>
          <li>Install matching WebMux versions on both computers, plus a native USB exporter beside
            the device and its compatible importer on the receiving computer.
            {' '}<a href="https://www.virtualhere.com/" target="_blank" rel="noreferrer">VirtualHere</a>
            {' '}offers both roles for these platforms and is obtained separately. Hardware compatibility is not yet verified by WebMux.</li>
          <li>Share only the intended devices in the exporter and restrict its network access to loopback.
            The tunnel exposes every device that exporter shares.</li>
          <li>Verify SSH key or agent access and the receiving computer’s host key first.
            OpenSSH and WebMux must be on PATH; the receiving computer needs an SSH server.</li>
        </ol>
        <fieldset className="usb-forward-fields" disabled={busy} onChange={() => { setCopyStatus(''); setTicket(null); }}>
          <label>Receiving SSH host
            <input data-dialog-autofocus value={host} onChange={e => setHost(e.target.value)}
              placeholder="developer@build-host" autoCapitalize="none" autoCorrect="off" spellCheck={false}
              aria-invalid={host !== '' && !validHost} aria-describedby="usb-host-hint" />
          </label>
          <small id="usb-host-hint">Hostname, IPv4 address or SSH alias, optionally user@host.</small>
          <div className="usb-forward-ports">
            <label>SSH port<input value={sshPort} inputMode="numeric" onChange={e => setSSHPort(e.target.value)} aria-invalid={!validPort(sshPort, 1)} /></label>
            <label>Local exporter port<input value={exportPort} inputMode="numeric" onChange={e => setExportPort(e.target.value)} aria-invalid={!validPort(exportPort, 1024)} /></label>
            <label>Lease<select value={minutes} onChange={e => setMinutes(e.target.value)}>
              <option value="15">15 minutes</option><option value="30">30 minutes</option><option value="60">1 hour</option>
            </select></label>
          </div>
        </fieldset>
        <button type="button" disabled={!valid || busy} onClick={() => void createCommand()}>Create local command</button>
        {error && <p role="alert">{error}</p>}
        <p>Run this in a <strong>local shell on the computer hosting the USB device or simulator</strong>.
          A WebMux terminal on the receiving computer is the wrong end.</p>
        <textarea ref={commandRef} aria-label="USB forwarding command" readOnly rows={3} value={command}
          placeholder="Create a local command to pair the helper with this dashboard." />
        <button type="button" disabled={!commandUsable} onClick={() => void copyCommand()}>Copy local command</button>
        <p role="status">{copyStatus}</p>
        <p>When the command prints a ready address, connect the native importer on the receiving
          computer to that <code>127.0.0.1:PORT</code> and select the device. Release it in the importer,
          then press Ctrl-C in the local shell to disconnect. Lease expiry also disconnects it.</p>
        <p>Use compatible native USB software for your robot, USB security key or other peripheral.
          A simulated USB device can be used for testing.</p>
        <h3 ref={statusRef} tabIndex={-1}>Forwarding status</h3>
        {unavailable && <p>Cannot refresh forwarding status. Check the local helper before assuming forwarding has stopped.</p>}
        <ul className="usb-forward-leases" aria-label="USB forwarding leases">
          {leases.map(lease => <li key={lease.id}>
            <strong>{lease.spec.host}</strong>
            <small className="usb-forward-lease-details">Exporter 127.0.0.1:{lease.spec.export_port} · SSH {lease.spec.ssh_port} · Lease {lease.id.slice(0, 8)} · Expires {new Date(lease.expires_at).toLocaleTimeString()}</small>
            <p>{leaseStatus(lease, unavailable)}</p>
            {!['ended', 'failed', 'uncertain', 'expired'].includes(lease.phase) &&
              <button type="button" disabled={busy || lease.stop_requested}
                aria-label={`Stop forwarding to ${lease.spec.host}`}
                onClick={() => void stopForwarding(lease.id)}>Stop forwarding</button>}
          </li>)}
        </ul>
        {leases.length === 0 && <p>No forwarding leases. Create a command and run it locally to begin.</p>}
        <p className="usb-forward-note">Creating a command does not start forwarding. The one-use pairing expires after five minutes.
          Status describes the helper’s transport; device attachment is verified in the native importer.</p>
      </div>
    </Dialog>
  );
}
