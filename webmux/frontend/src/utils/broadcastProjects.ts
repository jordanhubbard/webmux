import type { Session } from '../types';
import { executionHostLabel } from './sessionIdentity';

const provider = (session: Session) => session.rig?.provider || 'rig';
const ownerLabel = (session: Session) => {
  const name = provider(session) === 'agent-deck' ? 'Agent Deck' : provider(session) === 'rig' ? 'Rig' : provider(session);
  return session.rig?.manager_profile ? `${name} (${session.rig.manager_profile})` : name;
};
const recipientBase = (session: Session) => session.rig
  ? `${session.rig.project}/${session.rig.name} · ${ownerLabel(session)} · ${executionHostLabel(session.rig.host)}` : session.title;

export function broadcastRecipientLabels(sessions: Session[]) {
  const counts = new Map<string, number>();
  for (const session of sessions) counts.set(recipientBase(session), (counts.get(recipientBase(session)) || 0) + 1);
  return new Map(sessions.map(session => {
    const base = recipientBase(session);
    return [session.id, counts.get(base)! > 1 ? `${base} · viewer ${session.id}` : base];
  }));
}

// Labels are presentation only. Legacy/unknown records remain individually
// selectable; only coordinator-observed namespace and destination form a group.
export function broadcastProjects(sessions: Session[]) {
  const groups = new Map<string, { key: string; label: string; identity: string; ids: string[] }>();
  for (const session of sessions) {
    const rig = session.rig;
    if (!rig?.project_id || (!rig.host && !rig.rig_host_id) || !['rig', 'agent-deck'].includes(provider(session))) continue;
    const key = JSON.stringify([provider(session), rig.manager_profile || '', rig.project_id, rig.host, rig.rig_host_id || '']);
    const group = groups.get(key);
    if (group) group.ids.push(session.id);
    else groups.set(key, { key, label: `Project: ${rig.project} · ${ownerLabel(session)} · ${executionHostLabel(rig.host)}`,
      identity: `${rig.project_id}${rig.rig_host_id ? ` · Rig host ${rig.rig_host_id}` : ''}`, ids: [session.id] });
  }
  const values = [...groups.values()];
  return values.map(group => ({ ...group, label: values.filter(other => other.label === group.label).length > 1 ? `${group.label} · ${group.identity}` : group.label }))
    .sort((a, b) => a.label.localeCompare(b.label));
}
