import { useState } from 'react';
import { copyDesktopText } from '../utils/terminalClipboard';
import { Dialog } from './Dialog';

export const MAX_CLIPBOARD_BYTES = 128 * 1024;

/** Clipboard access always follows a click; plain text remains a fallback. */
export function ClipboardDialog({ initialText = '', onSend, onClose }: {
  initialText?: string; onSend: (text: string) => void; onClose: () => void;
}) {
  const [text, setText] = useState(initialText);
  const [message, setMessage] = useState('');
  const access = async (copy: boolean) => {
    try {
      if (copy) { if (!await copyDesktopText(text)) throw new Error('unavailable'); }
      else { if (!navigator.clipboard) throw new Error('unavailable'); setText(await navigator.clipboard.readText()); }
      setMessage(copy ? 'Copied.' : 'Clipboard loaded. Review the text before sending.');
    } catch {
      setMessage('Clipboard access is unavailable or denied. Select and copy the text, or paste into the text box using your browser shortcut.');
    }
  };
  return <Dialog title="Clipboard" onClose={onClose}>
    <textarea aria-label="Clipboard text" data-dialog-autofocus value={text}
      onChange={e => setText(e.target.value)} style={{ width: '100%', minHeight: 160 }} />
    <p role="status">{message}</p>
    <button onClick={() => void access(true)}>Copy text</button>
    <button onClick={() => void access(false)}>Read clipboard</button>
    <button onClick={() => {
      if (new TextEncoder().encode(text).length > MAX_CLIPBOARD_BYTES) {
        setMessage('Send at most 128 KiB at a time.'); return;
      }
      try { onSend(text); onClose(); }
      catch (error) { setMessage((error as Error).message); }
    }}>Send text</button>
  </Dialog>;
}
