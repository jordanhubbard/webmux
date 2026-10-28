import { test, expect } from '@playwright/test';
import path from 'node:path';

test('project labels survive cwd changes and reloads, and browser windows follow their focused session', async ({ page, request }) => {
  const ids: string[] = [];
  try {
    for (const username of ['reviewer', 'builder']) {
      const response = await request.post('/api/sessions', { data: {
        hostname: 'localhost', username, transport: 'exec',
        exec_command: `"${process.execPath}" "${path.resolve(__dirname, 'project-fixture.mts')}"`,
      } });
      expect(response.ok()).toBe(true);
      const id = (await response.json()).id as string;
      ids.push(id);
      expect((await request.patch(`/api/sessions/${id}`, { data: { title: username } })).ok()).toBe(true);
    }
    expect((await request.patch(`/api/sessions/${ids[1]}`, { data: { project_label: 'Other project' } })).ok()).toBe(true);
    await page.goto('/');
    const tile = page.getByTestId(`tile-cell-${ids[0]}`);
    await expect(tile.locator('.xterm-rows')).toContainText('PROJECT_FIXTURE_READY');
    await tile.getByRole('button', { name: 'Set project', exact: true }).click();
    const editor = page.getByRole('dialog', { name: 'Project label', exact: true });
    await editor.getByLabel('Project name').fill('WebMux');
    await editor.getByRole('button', { name: 'Save project label' }).click();
    await expect(editor).toHaveCount(0);
    await expect(tile.getByText('WebMux / reviewer', { exact: true })).toBeVisible();
    await tile.locator('.xterm-helper-textarea').focus();
    await page.keyboard.type('change-directory\n');
    await expect(tile.locator('.xterm-rows')).toContainText('CWD_CHANGED');
    await expect(page).toHaveTitle(/WebMux \/ reviewer/);
    const sessions = await (await request.get('/api/sessions')).json() as { id: string; project_label?: string }[];
    expect(sessions.find(session => session.id === ids[0])?.project_label).toBe('WebMux');
    const other = page.getByTestId(`tile-cell-${ids[1]}`);
    await expect(other.locator('.xterm-rows')).toContainText('PROJECT_FIXTURE_READY');
    await other.locator('.xterm-helper-textarea').focus();
    await expect(page).toHaveTitle(/Other project \/ builder/);
    await page.reload();
    await expect(page).toHaveTitle(/Other project \/ builder/);
    await expect(tile.getByText('WebMux / reviewer', { exact: true })).toBeVisible();
  } finally {
    for (const id of ids) await request.delete(`/api/sessions/${id}`);
  }
});
