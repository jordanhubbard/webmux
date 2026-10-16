import { spawnSync } from 'node:child_process';

if (!process.stdin.isTTY) throw new Error('Clipboard fixture requires a PTY');
process.stdin.setRawMode(true);
process.stdin.resume();
process.stdout.write('\x1b[?2004hCLIPBOARD_READY\r\n');
process.stdin.on('data', (chunk: Buffer) => {
  process.stdout.write(`INPUT_HEX:${chunk.toString('hex')};\r\n`);
  if (chunk.includes(4)) process.stdout.write(`\x1b]52;c;${Buffer.from('copied from terminal 😀').toString('base64')}\x1b\\`);
  if (chunk.includes(5)) process.stdout.write('\x1b]52;c;?\x07');
  if (chunk.includes(6) && process.env.TMUX_PANE) {
    const run = (args: string[], input?: string) => {
      const result = spawnSync('tmux', args, { input, encoding: 'utf8', timeout: 5000 });
      if (result.error || result.status !== 0) throw new Error(`tmux clipboard fixture failed: ${result.error ?? result.stderr}`);
      return result.stdout.trim();
    };
    const session = run(['display-message', '-p', '-t', process.env.TMUX_PANE, '#{session_id}']);
    const client = run(['list-clients', '-t', session, '-F', '#{client_name}']).split('\n')[0];
    if (!client) throw new Error('tmux clipboard fixture has no attached client');
    // Codex uses this native tmux forwarding path alongside its host clipboard write.
    run(['load-buffer', '-w', '-t', client, '-'], 'copied through tmux');
  }
});
