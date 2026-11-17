import { useEffect, useState } from 'react';
import type { CoordinationSeat } from './CoordinationDialog';
import type { Session } from '../types';

const sources: Record<string, string> = {
  runtime_hook: 'runtime hook', pane_heuristic: 'pane estimate', terminal_activity: 'terminal output',
  tmux_session: 'tmux observation', external_cli: 'external CLI', session_registry: 'session registry',
};

export function RigSeatStatus({ session, observations = [] }: { session: Session; observations?: CoordinationSeat[] }) {
  const rig = session.rig;
  const matches = observations.filter(seat => seat.provider === 'rig' && seat.id === rig?.seat_id);
  // A stable seat can have a new occupant. Never attribute its observations to
  // an old viewer which has not explicitly opened the new native binding.
  const candidate = matches.length === 1 ? matches[0] : undefined;
  const seat = candidate && (!candidate.observationState || candidate.observationState === 'current') ? candidate : undefined;
  const bindingMatches = !!seat && rig?.target === seat.managerTarget && rig?.attachment_target === seat.attachmentTarget;
  const activity = bindingMatches ? seat?.agentActivity : undefined;
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    setNow(Date.now());
    const sampled = Date.parse(activity?.sampledAt || '');
    if (!Number.isFinite(sampled) || sampled > Date.now() + 5000 || sampled + 30000 <= Date.now()) return;
    const timer = setTimeout(() => setNow(Date.now()), Math.max(0, sampled + 30000 - Date.now()) + 1);
    return () => clearTimeout(timer);
  }, [activity]);
  if (!rig || rig.provider === 'agent-deck') return null;
  const sampled = Date.parse(activity?.sampledAt || '');
  const age = now - sampled;
  const fresh = activity && !activity.stale && activity.generationProvenance !== 'unresolved' && (activity.evidenceSource !== 'runtime_hook' || activity.generationProvenance === 'resolved')
    && Number.isFinite(age) && age >= -5000 && age < 30000;
  const source = sources[activity?.evidenceSource || ''] || 'unknown source';
  let label = 'Agent activity unknown';
  if (fresh && source !== 'unknown source') {
    if (activity.evidenceSource === 'runtime_hook') {
      label = ({ running: 'Agent running', needs_input: 'Agent needs input', idle: 'Agent idle' } as Record<string, string>)[activity.state] || label;
    } else if (activity.evidenceSource === 'terminal_activity' && activity.state === 'running') label = 'Terminal output observed';
    else if (activity.state !== 'unknown' && ['running', 'idle', 'needs_input'].includes(activity.state)) label = `Activity estimate: ${activity.state.replace(/_/g, ' ')}`;
  }
  const startup = ({ pending: 'pending', ready: 'ready', attention_required: 'needs attention', failed: 'failed' } as Record<string, string>)[bindingMatches ? seat?.startupStatus || '' : ''] || 'unknown';
  const enrollment = bindingMatches && seat?.enrollmentSource === 'rig-session-registry' ? ({ launched: 'Launched by Rig', claimed: 'Adopted by Rig' } as Record<string, string>)[seat.enrollmentOrigin || ''] || 'unknown' : 'unknown';
  const attention = seat?.state === 'attention_required' || (bindingMatches && (seat?.startupStatus === 'attention_required' || seat?.startupStatus === 'failed'))
    || (fresh && activity.evidenceSource === 'runtime_hook' && activity.state === 'needs_input');
  return <div aria-label="Rig seat status" style={{ fontSize: 11, padding: '2px 8px', background: '#17172c', color: attention ? '#f1d494' : '#b5adc9', flexShrink: 0 }}
    title="Rig observations; bootstrap completion and terminal output do not establish readiness for instructions. Viewer connection is shown separately.">
    <span title="Manager-reported enrollment lineage; this does not establish remote provider ownership or authorize Stop.">Enrollment {enrollment}</span>{' · '}
    <span>Rig {seat ? seat.state.replace(/_/g, ' ') : 'state unknown'}</span>{' · '}
    <span>Bootstrap {startup}</span>{' · '}
    <span>{label} ({source}{activity && !fresh ? '; stale or unverified' : ''})</span>
    {attention && <strong> · Rig attention</strong>}
  </div>;
}
