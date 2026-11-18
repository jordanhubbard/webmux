// Runner contract controls only; live SSH/Codex evidence is retained separately.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
const runner = path.resolve(import.meta.dirname, '../webmux/server/internal/openrigcoord/runner.mts');
for (const [host, runtime, modern, mismatch] of [['worker', 'codex', false, false], ['localhost', 'codex', false, false], ['worker', 'claude-code', false, false], ['legacy', 'codex', false, false], ['localhost', 'codex', true, false], ['localhost', 'codex', true, true]] as const) {
  test(`relay registration and launch grammar for ${host}/${runtime} modern=${modern} mismatch=${mismatch}`, { skip: process.platform === 'win32' ? 'Executes the POSIX shell/tmux launcher; qualified on macOS and Linux.' : false }, () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'wm-relay-contract-'));
    const bin = path.join(dir, 'bin'); fs.mkdirSync(bin);
    const stub = `#!${process.execPath}
const fs=require('node:fs'),path=require('node:path'),cp=require('node:child_process');
const name=path.basename(process.argv[1]),a=process.argv.slice(2),root=process.env.WM_RELAY_TEST;
fs.appendFileSync(path.join(root,'calls'),JSON.stringify([name,...a])+'\\n');
if(name==='rig'){
 if(process.env.OPENRIG_HOST_SELECTED!=='local'){console.error('wrong selected host');process.exit(8);}
 if(a[0]==='--version'){console.log(process.env.WM_MODERN==='1'?'0.7.18 (3bf87c62)':'0.6.14 (df1c1517)');process.exit(0);}
 if(a[0]==='up') console.log(JSON.stringify({rigId:'01M5H000000000000000000000'}));
 if(a[0]==='ps') console.log(JSON.stringify(a.includes('--nodes')?[{rigId:'01M5H000000000000000000000',podNamespace:'agents',logicalId:'agents.previous'}]:[{rigId:'01M5H000000000000000000000',name:'test',rigName:'test',lifecycleState:'running'}]));
 if(a[0]==='attach'&&a.includes('--help')){console.log(process.env.WM_LEGACY==='1'?'--self --rig':'--self --rig --relay-ssh-host --relay-tmux-session');process.exit(0);}
 if(a[0]==='attach') {const tmux=process.env.TEST_TMUX;console.log(JSON.stringify({ok:true,attachmentType:'tmux',sessionName:process.env.WM_MISMATCH==='1'?'another-physical-session':tmux,senderAddress:'canonical@test',sessionId:'private-session',env:{OPENRIG_URL:'http://127.0.0.1:4567',OPENRIG_SESSION_NAME:'canonical@test'}}));}
}else if(name==='codex'||name==='claude'){
 if(process.env.OPENRIG_HOST_SELECTED!=='local'||process.env.OPENRIG_URL!=='http://127.0.0.1:4567'||process.env.OPENRIG_SESSION_NAME!=='canonical@test')process.exit(9);
}else if(name==='ssh'){
 if(a.at(-1).includes('socket.socket')) console.log('3456');
 else if(a.at(-1).includes('command -v')) console.log('/tools/codex\\n/tools/rig\\n/tools/tmux');
}else if(name==='tmux'){
 if(a[0]==='-V'){console.log('tmux 3.4');process.exit(0);}
 if(a[0]==='has-session') process.exit(fs.existsSync(path.join(root,'alive'))?0:1);
 if(a[0]==='new-session'){const t=a[a.indexOf('-s')+1];fs.writeFileSync(path.join(root,'alive'),t);const r=cp.spawnSync('/bin/sh',['-c',a.at(-1)],{env:{...process.env,TEST_TMUX:t,OPENRIG_HOST_SELECTED:'webmux-qa-shadow'},encoding:'utf8'});if(r.status)process.stderr.write(r.stderr);process.exit(r.status||0);}
}
`;
    for (const name of ['rig', 'ssh', 'tmux', 'codex', 'claude']) fs.writeFileSync(path.join(bin,name),stub,{mode:0o700});
    if (modern) {
      // This control covers the earlier managed-launch record migration and
      // relay grammar, not native topology allocation (qualified separately).
      fs.mkdirSync(path.join(dir, 'test'));
      fs.writeFileSync(path.join(dir, 'test', 'project.json'), JSON.stringify({ rigId: '01M5H000000000000000000000' }), { mode: 0o600 });
    }
    try {
      const result = spawnSync(process.execPath,[runner,'create',dir],{input:JSON.stringify({project:'test',name:'agent',host,cwd:dir,runtime}),env:{...process.env,WM_RELAY_TEST:dir,WM_MODERN:modern?'1':'0',WM_MISMATCH:mismatch?'1':'0',OPENRIG_HOST_SELECTED:'webmux-qa-shadow',WM_LEGACY:host==='legacy'?'1':'0',PATH:`${bin}:${process.env.PATH}`},encoding:'utf8',timeout:20000});
      if(mismatch){
        assert.notEqual(result.status,0);
        const calls=fs.readFileSync(path.join(dir,'calls'),'utf8').trim().split('\n').map(line=>JSON.parse(line) as string[]);
        assert(!calls.some(a=>a[0]==='codex'));
        return;
      }
      assert.equal(result.status,0,result.stdout+result.stderr);
      const registered=JSON.parse(result.stdout) as {target:string;tmux:string};
      assert.equal(registered.target,modern?registered.tmux:'canonical@test');
      const calls=fs.readFileSync(path.join(dir,'calls'),'utf8').trim().split('\n').map(line=>JSON.parse(line) as string[]);
      if (host === 'localhost' && runtime === 'codex') {
        const coding = calls.filter(a => a[0] === 'codex');
        assert.equal(coding.length, 1);
        // Only the identity prompt is supplied; permissions come from native
        // owner configuration rather than a WebMux network/sandbox override.
        assert.equal(coding[0].length, 2); assert.match(coding[0][1], /^You are agent,/);
      }
      const attach=calls.find(a=>a[0]==='rig'&&a[1]==='attach'&&!a.includes('--help'))!;
      const expected=host!=='localhost'&&host!=='legacy'&&runtime==='codex';
      assert.equal(attach.includes('--relay-ssh-host'),expected);
      if(expected){
        assert.equal(attach[attach.indexOf('--relay-ssh-host')+1],host);
        const session=attach[attach.indexOf('--relay-tmux-session')+1];
        const remote=calls.filter(a=>a[0]==='ssh');
        assert(remote.some(a=>a.at(-1)!.includes('tmux new-session -d')));
        assert.equal(remote.at(-1)!.at(-1),`export PATH="$PATH:$HOME/.local/bin:/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin"; tmux set-option -t '${session}' mouse on && exec tmux attach-session -t '=${session}'`);
        assert(remote.every(a=>a.includes('StrictHostKeyChecking=yes')));
      }
      if (host !== 'localhost') {
        const saved=fs.readdirSync(dir).find(name=>name.endsWith('.remote.json'))!;
        const commands=JSON.parse(fs.readFileSync(path.join(dir,saved),'utf8')) as {setup:string;run:string};
        assert(commands.setup.includes('export OPENRIG_HOST_SELECTED=local'));
        if (runtime === 'codex') {
          assert(commands.setup.includes('codex '));
          assert.equal(commands.setup.includes('sandbox_workspace_write'), false);
        }
        assert.match(commands.setup,/tmux set-option -t 'wm-[a-z0-9-]+' mouse on/);
        assert.match(commands.run,/tmux set-option -t 'wm-[a-z0-9-]+' mouse on && exec tmux attach-session/);
        if (!expected) assert.match(calls.filter(a=>a[0]==='ssh').at(-1)!.at(-1)!,/tmux set-option -t 'wm-[a-z0-9-]+' mouse on/);
      }
    } finally { fs.rmSync(dir,{recursive:true,force:true}); }
  });
}
