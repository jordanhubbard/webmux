import { useEffect } from 'react';
import { it, expect, vi } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { RdpFullscreen } from '@frontend/components/RdpFullscreen';
import type { RdpSession } from '@frontend/types';

const fixture = vi.hoisted(() => ({ ack: null as null | ((status: { isError: () => boolean; message: string }) => void) }));
vi.mock('guacamole-common-js', () => ({ default: {
  ArrayBufferWriter: class {
    set onack(callback: typeof fixture.ack) { fixture.ack = callback; }
    sendData() {} sendEnd() {}
  },
} }));
vi.mock('@frontend/components/RdpViewer', () => ({
  RdpViewer: ({ clientRef, onStateChange }: { clientRef: { current: unknown }; onStateChange: (state: string) => void }) => {
    useEffect(() => {
      clientRef.current = { createClipboardStream: () => ({}) };
      onStateChange('connected');
    }, [clientRef, onStateChange]);
    return null;
  },
}));
it('shows a desktop clipboard rejection even after the send dialog closes', () => {
  render(<RdpFullscreen session={{ id: 'rdp', title: 'Desktop' } as RdpSession} onBack={vi.fn()} onDisconnect={vi.fn()} />);
  fireEvent.click(screen.getByTitle('RDP Options'));
  fireEvent.click(screen.getByText('Clipboard'));
  fireEvent.change(screen.getByLabelText('Clipboard text'), { target: { value: 'test' } });
  fireEvent.click(screen.getByText('Send text'));
  act(() => fixture.ack?.({ isError: () => true, message: 'Clipboard disabled by remote policy' }));
  expect(screen.getByRole('alert')).toHaveTextContent('Desktop rejected clipboard text');
  expect(screen.getByRole('alert')).toHaveTextContent('Clipboard disabled by remote policy');
});
