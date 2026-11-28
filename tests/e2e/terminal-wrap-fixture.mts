if (!process.stdin.isTTY) throw new Error('Wrap-copy fixture requires a PTY');
process.stdin.setRawMode(true);
process.stdin.resume();
process.stdout.write('WRAP_COPY_READY');
const logical = 'WRAP_BEGIN ' + 'alpha beta gamma '.repeat(22) + ' WRAP_END';
process.stdin.on('data', (data: Buffer) => {
  if (data.includes(7)) {
    const width = process.stdout.columns;
    const displayed = process.argv.includes('--prewrapped')
      ? Array.from({ length: Math.ceil(logical.length / width) }, (_, row) => logical.slice(row * width, (row + 1) * width)).join('\r\n')
      : logical;
    process.stdout.write('\x1b[?7h\x1b[2J\x1b[H' + displayed + '\r\nREAL_NEWLINE');
  }
  // Model an application's source-text copy channel without terminal wrapping.
  if (data.includes(15)) process.stdout.write('\x1b]52;c;' + Buffer.from(logical + '\nREAL_NEWLINE').toString('base64') + '\x07');
});
