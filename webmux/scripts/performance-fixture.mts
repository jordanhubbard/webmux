// Local child for backend latency/throughput measurements; no network access.
import { createInterface } from 'node:readline';
import { once } from 'node:events';

const input = createInterface({ input: process.stdin, terminal: false });
console.log('benchmark-ready');
for await (const line of input) {
  if (line.startsWith('ping-')) console.log(`pong-${line.slice(5)}:done`);
  else if (line === 'bulk') {
    const chunk = 'x'.repeat(4096);
    for (let i = 0; i < 256; i++) {
      if (!process.stdout.write(chunk)) await once(process.stdout, 'drain');
    }
    console.log('\nbenchmark-bulk-done');
  }
}
