import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import { RigSeatStatus } from '@frontend/components/RigSeatStatus';
import type { CoordinationSeat } from '@frontend/components/CoordinationDialog';
import type { Session } from '@frontend/types';

const session = { rig: { provider: 'rig', seat_id: 'local-rig/stable', target: 'native-first', attachment_target: 'first' } } as Session;
function observation(patch: Partial<CoordinationSeat> = {}): CoordinationSeat {
  return { provider: 'rig', id: 'local-rig/stable', managerTarget: 'native-first', attachmentTarget: 'first', state: 'running', startupStatus: 'ready',
    agentActivity: { state: 'needs_input', evidenceSource: 'runtime_hook', sampledAt: new Date().toISOString(), generationProvenance: 'resolved' }, ...patch } as CoordinationSeat;
}
afterEach(() => vi.useRealTimers());
describe('Rig tile observations', () => {
  it('separates bootstrap from fresh runtime-hook input attention', () => {
    render(<RigSeatStatus session={session} observations={[observation()]} />);
    expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Rig running · Attachment unknown · Bootstrap ready · Agent needs input (runtime hook) · Rig attention');
  });
  it('does not call terminal motion agent readiness or work', () => {
    render(<RigSeatStatus session={session} observations={[observation({ agentActivity: { state: 'running', evidenceSource: 'terminal_activity', sampledAt: new Date().toISOString() } })]} />);
    expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Terminal output observed (terminal output)');
    expect(screen.getByLabelText('Rig seat status')).not.toHaveTextContent('Agent running');
  });
  for (const patch of [ { stale: true }, { generationProvenance: undefined }, { generationProvenance: 'unresolved' }, { sampledAt: 'invalid' }, { sampledAt: '2099-01-01T00:00:00Z' } ]) {
    it(`keeps unverified activity unknown: ${JSON.stringify(patch)}`, () => {
      const seat = observation(); seat.agentActivity = { ...seat.agentActivity!, ...patch };
      render(<RigSeatStatus session={session} observations={[seat]} />);
      expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Agent activity unknown');
      expect(screen.getByLabelText('Rig seat status')).not.toHaveTextContent('Rig attention');
    });
  }
  it('expires positive activity without another response', async () => {
    vi.useFakeTimers();
    render(<RigSeatStatus session={session} observations={[observation()]} />);
    expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Agent needs input');
    await act(() => vi.advanceTimersByTimeAsync(30001));
    expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Agent activity unknown');
  });
  it('refuses reattribution after a stable seat rebind and accepts explicit viewer rebinding', () => {
    const seat = observation({ managerTarget: 'native-second', attachmentTarget: 'second' });
    const view = render(<RigSeatStatus session={session} observations={[seat]} />);
    expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Rig running · Attachment unknown · Bootstrap unknown · Agent activity unknown');
    view.rerender(<RigSeatStatus session={{ ...session, rig: { ...session.rig!, target: 'native-second', attachment_target: 'second' } }} observations={[seat]} />);
    expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Agent needs input');
  });
  it('rejects duplicate canonical identities and old offline observations', () => {
    const view = render(<RigSeatStatus session={session} observations={[observation(), observation()]} />);
    expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Rig state unknown');
    view.rerender(<RigSeatStatus session={session} observations={[observation({ observationState: 'offline' })]} />);
    expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Rig state unknown');
  });
  it('requires explicit viewer rebinding when only the tmux socket changes', () => {
    const bound = { ...session, rig: { ...session.rig!, tmux_server_flag: '-S', tmux_server_value: '/tmp/first' } };
    const seat = observation({ tmuxServerFlag: '-S', tmuxServerValue: '/tmp/second' });
    const view = render(<RigSeatStatus session={bound} observations={[seat]} />);
    expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Bootstrap unknown · Agent activity unknown');
    view.rerender(<RigSeatStatus session={{ ...bound, rig: { ...bound.rig, tmux_server_value: '/tmp/second' } }} observations={[seat]} />);
    expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Agent needs input');
  });
  it('leaves ordinary and Agent Deck viewers alone', () => {
    const view = render(<RigSeatStatus session={{} as Session} />);
    expect(screen.queryByLabelText('Rig seat status')).toBeNull();
    view.rerender(<RigSeatStatus session={{ rig: { ...session.rig!, provider: 'agent-deck' } } as Session} />);
    expect(screen.queryByLabelText('Rig seat status')).toBeNull();
  });
});

it('distinguishes manager-reported enrollment from a WebMux launch record or lifecycle authority', () => {
  const seat = observation({ managed: false, enrollmentOrigin: 'launched', enrollmentSource: 'rig-session-registry' });
  const view = render(<RigSeatStatus session={session} observations={[seat]} />);
  expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Enrollment Launched by Rig');
  view.rerender(<RigSeatStatus session={session} observations={[{ ...seat, managed: true, enrollmentOrigin: 'claimed' }]} />);
  expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Enrollment Adopted by Rig');
  view.rerender(<RigSeatStatus session={session} observations={[{ ...seat, enrollmentSource: 'unavailable' }]} />);
  expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Enrollment unknown');
});

it('keeps attachment availability separate from lifecycle, activity and the saved viewer binding', () => {
  const view = render(<RigSeatStatus session={session} observations={[observation({ transportState: 'unavailable' })]} />);
  expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Rig running · Attachment unavailable · Bootstrap ready');
  expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Agent needs input');
  view.rerender(<RigSeatStatus session={session} observations={[observation({ transportState: 'available' })]} />);
  expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Attachment available');
  view.rerender(<RigSeatStatus session={session} observations={[observation({ transportState: 'reported' })]} />);
  expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Attachment reported');
  for (const patch of [
    { transportState: undefined },
    { transportState: 'unexpected' },
    { transportState: 'available', observationState: 'offline' },
    { transportState: 'available', managerTarget: 'replacement' },
    { transportState: 'available', tmuxServerFlag: '-S', tmuxServerValue: '/tmp/replacement' },
  ]) {
    view.rerender(<RigSeatStatus session={session} observations={[observation(patch)]} />);
    expect(screen.getByLabelText('Rig seat status')).toHaveTextContent('Attachment unknown');
    expect(screen.getByLabelText('Rig seat status')).not.toHaveTextContent('Attachment available');
  }
});
