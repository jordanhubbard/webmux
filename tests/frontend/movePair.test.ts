import { expect, it, vi } from 'vitest';
import { movePair } from '@frontend/utils/movePair';
it('compensates the first move when the second fails', async () => {
  const undo = vi.fn().mockResolvedValue(undefined);
  await expect(movePair(vi.fn().mockResolvedValue(undefined), vi.fn().mockRejectedValue(new Error('save failed')), undo)).rejects.toThrow('save failed');
  expect(undo).toHaveBeenCalledOnce();
});
it('reports partial persistence when compensation also fails', async () => {
  await expect(movePair(vi.fn().mockResolvedValue(undefined), vi.fn().mockRejectedValue(new Error()), vi.fn().mockRejectedValue(new Error()))).rejects.toThrow('partially saved');
});
it('never moves the second item after the first fails', async () => {
  const second = vi.fn();
  const undo = vi.fn();
  await expect(movePair(vi.fn().mockRejectedValue(new Error('save failed')), second, undo)).rejects.toThrow();
  expect(second).not.toHaveBeenCalled();
  expect(undo).not.toHaveBeenCalled();
});
