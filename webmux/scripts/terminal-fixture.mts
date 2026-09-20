// Local interactive child for differential terminal protocol tests. It never
// opens a network connection or reads the operator's configuration.
import { createInterface } from 'node:readline';
import { closeSync, openSync } from 'node:fs';
import { WriteStream } from 'node:tty';

function currentWindowSize(): [number, number] {
  // getWindowSize() returns cached fields. Windows does not reliably refresh
  // process.stdout's cache for cooked input, even after ConPTY has resized.
  // A fresh, independently owned terminal descriptor queries the current size.
  const fd = openSync(process.platform === 'win32' ? String.raw`\\.\CONOUT$` : '/dev/tty', 'r+');
  let terminal: WriteStream;
  try { terminal = new WriteStream(fd); }
  catch (error) { closeSync(fd); throw error; }
  try { return terminal.getWindowSize(); }
  finally { terminal.destroy(); }
}

const input = createInterface({ input: process.stdin, terminal: false });
console.log('fixture-ready');
input.on('line', (line: string) => {
  if (line === 'exit') process.exit(0);
  if (line === 'size') {
    const [cols, rows] = currentWindowSize();
    console.log(`fixture-size:${cols}x${rows}`);
  } else {
    console.log(`fixture-reply:${line}:λ😀`);
  }
});
