import { useState } from 'react';
import { Dialog } from './Dialog';
import './USBForwardDialog.css';

export function USBForwardDialog({ onClose }: { onClose: () => void }) {
  const [host, setHost] = useState('');
  const [sshPort, setSSHPort] = useState('22');
  const [exportPort, setExportPort] = useState('7575');
  const [minutes, setMinutes] = useState('30');
  const [copyStatus, setCopyStatus] = useState('');
  const validPort = (value: string, minimum: number) => /^\d+$/.test(value)
    && Number(value) >= minimum && Number(value) <= 65535;
  // Deliberately match the CLI's restricted destination grammar. This command
  // must be safe to paste into POSIX shells, PowerShell and cmd.exe.
  const validHost = host.length <= 253
    && /^([a-zA-Z0-9_][a-zA-Z0-9_.-]*@)?[a-zA-Z0-9_][a-zA-Z0-9_.-]*$/.test(host);
  const valid = validHost && validPort(sshPort, 1) && validPort(exportPort, 1024);
  const command = valid
    ? `webmux usb-forward --host ${host} --ssh-port ${Number(sshPort)} --export-port ${Number(exportPort)} --duration ${minutes}m`
    : '';

  async function copyCommand() {
    try {
      await navigator.clipboard.writeText(command);
      setCopyStatus('Command copied. Run it on the computer holding the physical device.');
    } catch {
      setCopyStatus('Clipboard unavailable. Select and copy the command below.');
    }
  }

  return (
    <Dialog title="USB redirection" subtitle="Experimental · native USB software required" onClose={onClose} width={620}>
      <div className="usb-forward">
        <p className="usb-forward-route"><span aria-hidden="true">🔌</span> Physical device computer → SSH → Computer using the device</p>
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
        <div className="usb-forward-fields" onChange={() => setCopyStatus('')}>
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
        </div>
        <p>Run this in a <strong>local shell on the computer holding the physical device</strong>.
          A WebMux terminal on the receiving computer is the wrong end.</p>
        <textarea aria-label="USB forwarding command" readOnly rows={3} value={command}
          placeholder="Enter a valid host and ports to generate the command." />
        <button type="button" disabled={!valid} onClick={() => void copyCommand()}>Copy local command</button>
        <p role="status">{copyStatus}</p>
        <p>When the command prints a ready address, connect the native importer on the receiving
          computer to that <code>127.0.0.1:PORT</code> and select the device. Release it in the importer,
          then press Ctrl-C in the local shell to disconnect. Lease expiry also disconnects it.</p>
        <p>For iPhone development, the receiving computer must be a Mac with Xcode.
          Physical iPhone discovery, installation and debugging remain unverified.</p>
        <p className="usb-forward-note">Setup only: this dialog does not start a tunnel or monitor devices.
          Connection status is available in the local helper and native importer.</p>
      </div>
    </Dialog>
  );
}
