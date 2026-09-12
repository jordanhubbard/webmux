import { useEffect, useId, useRef, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import './Dialog.css';

interface DialogProps {
  title: string;
  children: ReactNode;
  onClose?: () => void;
  subtitle?: string;
  accessibleName?: string;
  width?: number;
  dismissible?: boolean;
}

/** The browser owns focus containment and makes the workspace inert while open. */
export function Dialog({ title, children, onClose, subtitle, accessibleName,
  width = 420, dismissible = true }: DialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const backdropPress = useRef(false);

  useEffect(() => {
    const dialog = dialogRef.current!;
    const opener = document.activeElement;
    dialog.showModal();
    // React's autoFocus runs before showModal and does not set the HTML attribute.
    dialog.querySelector<HTMLElement>('[data-dialog-autofocus]')?.focus();
    return () => {
      dialog.close();
      if (opener instanceof HTMLElement && opener.isConnected) {
        opener.focus({ preventScroll: true });
      }
    };
  }, []);

  const dismiss = () => {
    if (dismissible) onClose?.();
  };

  return createPortal(
    <dialog
      ref={dialogRef}
      className="webmux-dialog"
      aria-modal="true"
      aria-label={accessibleName}
      aria-labelledby={accessibleName ? undefined : titleId}
      onCancel={event => {
        event.preventDefault();
        dismiss();
      }}
      onPointerDown={event => { backdropPress.current = event.target === event.currentTarget; }}
      onClick={event => {
        // Dragging from a field onto the backdrop must not discard a form.
        if (event.target === event.currentTarget && backdropPress.current) dismiss();
        backdropPress.current = false;
      }}
    >
      <div className="webmux-dialog-panel" style={{ width }}>
        <header className="webmux-dialog-header">
          <div>
            <h2 id={titleId} className="webmux-dialog-title">{title}</h2>
            {subtitle && <p className="webmux-dialog-subtitle">{subtitle}</p>}
          </div>
          {onClose && (
            <button type="button" className="webmux-dialog-close" onClick={dismiss}
              disabled={!dismissible} aria-label={`Close ${title.toLowerCase()}`} title="Close (Esc)">
              ✕
            </button>
          )}
        </header>
        {children}
      </div>
    </dialog>,
    document.body,
  );
}
