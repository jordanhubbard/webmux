import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { TerminalScrollStrip } from '@frontend/components/TerminalScrollStrip';

describe('TerminalScrollStrip', () => {
  it('renders an up and a down arrow', () => {
    render(<TerminalScrollStrip onScroll={vi.fn()} />);
    expect(screen.getByLabelText('Scroll up')).toBeDefined();
    expect(screen.getByLabelText('Scroll down')).toBeDefined();
  });

  it('scrolls one page up when the up arrow is tapped', () => {
    const onScroll = vi.fn();
    render(<TerminalScrollStrip onScroll={onScroll} />);
    fireEvent.click(screen.getByLabelText('Scroll up'));
    expect(onScroll).toHaveBeenCalledWith(-1);
  });

  it('scrolls one page down when the down arrow is tapped', () => {
    const onScroll = vi.fn();
    render(<TerminalScrollStrip onScroll={onScroll} />);
    fireEvent.click(screen.getByLabelText('Scroll down'));
    expect(onScroll).toHaveBeenCalledWith(1);
  });
});
