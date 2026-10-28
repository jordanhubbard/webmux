// Real process whose cwd changes without changing its durable project binding.
import os from 'node:os';
import readline from 'node:readline';
console.log('PROJECT_FIXTURE_READY');
readline.createInterface({ input: process.stdin }).on('line', line => {
  if (line.trim() === 'change-directory') {
    process.chdir(os.tmpdir());
    console.log(`CWD_CHANGED ${process.cwd()}`);
  }
});
