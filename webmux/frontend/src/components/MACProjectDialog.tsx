import { useEffect, useState } from 'react';
import { api, type MACProjectView } from '../utils/api';
import { Dialog } from './Dialog';
import './MACProjectDialog.css';

/** Explicit reads only. A failed refresh or scope change removes old facts. */
export function MACProjectDialog({ sessionId, onClose }: { sessionId: string; onClose: () => void }) {
  const [result, setResult] = useState<{ session: string; offset: number; value: MACProjectView } | null>(null);
  const [failure, setFailure] = useState<{ session: string; message: string } | null>(null);
  const [offset, setOffset] = useState(0);
  const [refresh, setRefresh] = useState(0);
  const value = result?.session === sessionId && result.offset === offset ? result.value : null;
  const error = failure?.session === sessionId ? failure.message : '';
  useEffect(() => {
    const controller = new AbortController();
    setResult(null); setFailure(null);
    api.getMACProject(sessionId, offset, controller.signal).then(view => {
      if (!controller.signal.aborted) setResult({ session: sessionId, offset, value: view });
    }).catch((cause: unknown) => {
      if (!controller.signal.aborted) setFailure({ session: sessionId, message: cause instanceof Error ? cause.message : 'Cannot load project activity' });
    });
    return () => controller.abort();
  }, [sessionId, offset, refresh]);
  return <Dialog title="Project activity" onClose={onClose} width={840}>
    <div className="mac-project-content">
      <button type="button" onClick={() => { setResult(null); setFailure(null); setOffset(0); setRefresh(n => n + 1); }}>Refresh project</button>
      {error && <p role="alert">{error}</p>}
      {!value && !error && <p role="status">Loading project activity…</p>}
      {value && <>
        <p>MAC project: <strong>{value.project.project_key}</strong>. Observed {value.project.observed_at}.</p>
        <p>Coverage: {value.project.coverage.replace(/_/g, ' ')}. Showing {value.project.tasks.length} tasks from this page; MAC reports {value.project.reported_total} in total. Freshness has not been qualified.</p>
        <h3>What are agents doing?</h3>
        <p>MAC workers below are associated with the observed tasks. WebMux sessions are listed separately; their presence does not establish which task they are executing.</p>
        {value.project.tasks.length > 0 ? <div className="mac-project-table"><table>
          <thead><tr><th>Task</th><th>State / priority</th><th>Worker / machine</th><th>Attempt / lease observation</th></tr></thead>
          <tbody>{value.project.tasks.map(row => <tr key={row.task.task_id}>
            <td>{row.task.title}<br /><small>{row.task.task_id}</small></td>
            <td>{row.task.state}<br />Reported priority: {row.task.priority}</td>
            <td>{row.worker ? <>{row.worker.worker_name} ({row.worker.worker_id})<br />{row.worker.machine_hostname} ({row.worker.machine_id})<br />{row.worker.availability}</> : row.worker_state.replace(/_/g, ' ')}</td>
            <td>Attempt {row.task.attempt_count}<br />{row.task.lease_observation.replace(/_/g, ' ')}{row.task.leased_until && <><br />Until {row.task.leased_until}</>}</td>
          </tr>)}</tbody>
        </table></div> : <p>No tasks are visible on this page. This does not prove that the project has no work or agents.</p>}
        {(value.project.unavailable_tasks > 0 || value.project.unavailable_workers > 0 || value.project.adapter_omitted_tasks > 0 || value.project.upstream_omitted_tasks > 0) && <p role="status">Some tasks or worker observations are unavailable or omitted. This view is incomplete.</p>}
        <h3>WebMux sessions</h3>
        <ul>{value.sessions.map(item => <li key={`${item.session.node_id}:${item.session.session_id}:${item.session.incarnation}`}>
          {item.session.session_id} — {item.execution_host} · {item.transport} · {item.availability.replace(/_/g, ' ')}. MAC task link: {item.mac_link_state}. Observed {item.observed_at}.
        </li>)}</ul>
        <h3>Source context</h3>
        {value.source_state === 'observed' && value.source ? <>
          <p>Worktree: {value.source.worktree}<br />
            {value.source.detached ? 'Detached HEAD' : `Branch: ${value.source.ref.replace(/^refs\/heads\//, '')}`}<br />
            Commit: <code>{value.source.commit}</code><br />
            {value.source.dirty ? 'Uncommitted or untracked changes observed.' : 'No uncommitted or ordinary untracked changes observed.'} Observed {value.source.observed_at}.</p>
          <p>The commit identifies Git history, not the contents of uncommitted changes. This source observation is not linked to a MAC task or roadmap revision.</p>
        </> : <p>{value.source_state === 'execution_host_unavailable' ? 'Source inspection on this execution host is unavailable.' : 'Verified source context is unavailable.'} A displayed session path alone does not verify source identity.</p>}
        <h3>Source roadmap</h3>
        {value.roadmap_state === 'observed_committed_snapshot' && value.roadmap ? <>
          <p>{value.roadmap.path} at commit <code>{value.roadmap.commit}</code>. Observed {value.roadmap.observed_at}.</p>
          {value.roadmap.working_copy_dirty && <p role="status">Working-copy changes to this roadmap are not included in this committed snapshot.</p>}
          <details><summary>Read committed source roadmap</summary><pre aria-label="Source roadmap text" className="mac-project-roadmap">{value.roadmap.text}</pre></details>
          <p>These source requirements have not been linked to MAC tasks. This snapshot does not verify issue status or grant admission.</p>
        </> : <p>{value.roadmap_state === 'not_configured' ? 'Source roadmap access has not been configured.' : value.roadmap_state === 'execution_host_unavailable' ? 'Source roadmap observation on this execution host is unavailable.' : 'No verified roadmap snapshot is available.'}</p>}
        <h3>Are we duplicating work or conflicting?</h3>
        <p>Assessment: {value.project.conflict_verdict}. Task scope, source changes and ownership have not been reconciled. Standalone history is retained and unreconciled.</p>
        <h3>Are we working in roadmap order?</h3>
        <p>Assessment: {value.project.ordering_verdict}. The source roadmap is not yet linked to these tasks. Reported priorities and dependencies do not grant permission to start work.</p>
        {value.project.dependencies.length > 0 && <ul>{value.project.dependencies.map((edge, index) => <li key={index}>{edge.from_task_id} → {edge.to_task_id}: {edge.reported_verdict} ({edge.reported_join_policy})</li>)}</ul>}
        {value.project.withheld_dependencies > 0 && <p>Some dependencies are outside this authorized page or unavailable.</p>}
        <p>This view grants no work claim or lease. Project existence and session-to-worker links remain unverified.</p>
        {value.project.next_offset !== null && <button type="button" onClick={() => { setResult(null); setOffset(value.project.next_offset!); }}>Next task page</button>}
        {offset > 0 && <p>Each page is a fresh observation; tasks can move between pages.</p>}
      </>}
    </div>
  </Dialog>;
}
