if (!process.stdin.isTTY) throw new Error('Scroll fixture requires a PTY');
process.stdin.setRawMode(true);
process.stdin.resume();
if (process.argv[2] === 'alternate') process.stdout.write('\x1b[?1049hALT WHEEL READY');
else {
  for (let line = 0; line < 120; line++) process.stdout.write(`SCROLL-LINE-${line}\r\n`);
  process.stdout.write('SCROLL READY');
}
process.stdin.on('data', (data: Buffer) => process.stdout.write(`INPUT:${data.toString('hex')};`));
