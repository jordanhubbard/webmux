// Local interactive child for differential terminal protocol tests. It never
// opens a network connection or reads the operator's configuration.
import { createInterface } from 'node:readline';

const input = createInterface({ input: process.stdin, terminal: false });
console.log('fixture-ready');
input.on('line', (line: string) => {
  if (line === 'exit') process.exit(0);
  if (line === 'size') {
    const [cols, rows] = process.stdout.getWindowSize();
    console.log(`fixture-size:${cols}x${rows}`);
  } else {
    console.log(`fixture-reply:${line}:λ😀`);
  }
});
