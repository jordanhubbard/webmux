import { execFileSync } from 'node:child_process';
import { waitForRigChange } from './rig-changes.mts';
process.env.OPENRIG_HOST_SELECTED = 'local';
const run = (command: string, args: string[]) => execFileSync(command, args, {
  encoding: 'utf8', timeout: 2000, maxBuffer: 64 * 1024, stdio: ['ignore', 'pipe', 'pipe'],
});
console.log(JSON.stringify(await waitForRigChange(run, fetch, 20000, () => console.log('{"ready":true}'))));
