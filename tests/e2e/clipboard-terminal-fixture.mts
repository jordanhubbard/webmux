if (!process.stdin.isTTY) throw new Error('Clipboard fixture requires a PTY');
process.stdin.setRawMode(true);
process.stdin.resume();
process.stdout.write('\x1b[?2004hCLIPBOARD_READY\r\n');
process.stdin.on('data', (chunk: Buffer) => {
  process.stdout.write(`INPUT_HEX:${chunk.toString('hex')};\r\n`);
  if (chunk.includes(4)) process.stdout.write(`\x1b]52;c;${Buffer.from('copied from terminal 😀').toString('base64')}\x1b\\`);
  if (chunk.includes(5)) process.stdout.write('\x1b]52;c;?\x07');
});
