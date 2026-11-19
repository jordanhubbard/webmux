import type { Session } from '../types';

export const executionHostLabel = (host: string | undefined) => host || 'Unknown execution host';

// Project membership is durable session metadata, not a guess from a mutable cwd.
export function sessionIdentity(session: Session) {
  const project = session.rig?.project || session.project_label || '';
  const host = session.rig ? executionHostLabel(session.rig.host) : session.hostname || 'localhost';
  const originalTitle = session.rig ? `${session.rig.name} · ${host} · ${session.rig.project}` : '';
  const name = session.rig && session.title === originalTitle ? session.rig.name : session.title;
  const label = project ? `${project} / ${name}` : name;
  const source = session.rig ? (session.rig.provider === 'agent-deck' ? 'Agent Deck' : 'Rig') : session.project_label ? 'Saved project label' : 'Project not set';
  return { project, host, name, label, source, workspace: session.rig?.workspace || '' };
}
