import { test } from 'node:test';
import assert from 'node:assert/strict';
import { rigTmuxAttachment } from '../webmux/server/internal/openrigcoord/rig-attachment.mts';

const seat = { canonicalSessionName: 'agents.first@project', attachmentType: 'tmux' };
test('qualified structured attachments preserve literal sockets and normalize standard labels', () => {
  for (const [args, socket] of [[[], '/tmp/tmux-501/default'], [['-L', 'private'], '/tmp/tmux-501/private'],
    [['-S', "/private/tmp/a space/'literal'/socket"], "/private/tmp/a space/'literal'/socket"]] as const) {
    assert.deepEqual(rigTmuxAttachment({ ...seat, tmuxServerArgs: args }, true, 501), {
      tmuxTarget: seat.canonicalSessionName, tmuxServerFlag: '-S', tmuxServerValue: socket,
    });
  }
});
test('structured attachment refuses malformed selection rather than falling back to ambient tmux', () => {
  for (const args of [null, {}, 'tmux -S /tmp/x', ['-S'], ['-S', 'relative'], ['-L', '../other'], ['-L', '..'],
    ['-S', '/tmp/x', 'attach'], ['-f', '/tmp/x'], ['-S', '/tmp/a\nnext'], ['-S', '/tmp/a\u0085next']]) {
    assert.equal(rigTmuxAttachment({ ...seat, tmuxServerArgs: args }, true, 501), undefined, JSON.stringify(args));
  }
  for (const uid of [-1, NaN, 1.5]) assert.equal(rigTmuxAttachment(seat, true, uid), undefined);
  assert.equal(rigTmuxAttachment({ ...seat, attachmentType: 'external_cli' }, true, 501), undefined);
  for (const target of ['x;kill-server', 'x\nnext', 'x y', '', 'x'.repeat(257)]) {
    assert.equal(rigTmuxAttachment({ ...seat, canonicalSessionName: target }, true, 501), undefined);
  }
});
test('legacy contract admits only an exact canonical name and never interprets human shell text', () => {
  const node = { ...seat, tmuxAttachCommand: `tmux attach -t ${seat.canonicalSessionName}` };
  assert.deepEqual(rigTmuxAttachment(node, false), {
    tmuxTarget: seat.canonicalSessionName, tmuxServerFlag: '', tmuxServerValue: '',
  });
  for (const command of ['tmux attach -t different', `tmux -S /tmp/x attach -t ${seat.canonicalSessionName}`,
    `tmux attach -t ${seat.canonicalSessionName}; kill-server`]) {
    assert.equal(rigTmuxAttachment({ ...node, tmuxAttachCommand: command }, false), undefined);
  }
});
