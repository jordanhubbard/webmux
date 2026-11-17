import { expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { RigNavigator } from '@frontend/components/RigNavigator';

vi.mock('@frontend/utils/api', () => ({ api: { getCoordination: vi.fn(async () => ({
  projects: [{ provider: 'rig', id: 'project', name: 'remote project' }],
  seats: [{ provider: 'rig', projectId: 'project', id: 'host-rig/client/seat', name: 'worker', host: 'worker.example', runtime: 'codex', state: 'running', observationState: 'offline', transportState: 'offline', openable: false }],
})) } }));

it('keeps an offline host distinct from the last observed running seat in grid navigation', async () => {
  const open = vi.fn();
  render(<RigNavigator onOpen={open} onManage={() => {}} />);
  const seat = await screen.findByRole('button', { name: 'worker · worker.example · last observed running; offline · Rig' });
  expect(seat).toBeDisabled();
  expect(seat).toHaveAttribute('title', expect.stringContaining('attachment offline'));
  expect(open).not.toHaveBeenCalled();
});
