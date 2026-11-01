// A full-screen provider whose entry sequence falls outside the replay ring.
if (!process.stdin.isTTY) throw new Error('Replay fixture requires a PTY');
process.stdin.setRawMode(true);
process.stdin.resume();
process.stdout.write('\x1b[?1049h' + 'OLD HISTORY\r\n'.repeat(6000)
  + '\x1b[2J\x1b[HREPLAY READY\x1b[10;1HAGENT STATUS\x1b[11;1H\x1b[42mTMUX STATUS\x1b[0m');
