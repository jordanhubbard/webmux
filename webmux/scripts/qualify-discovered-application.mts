// Opt-in actual application/UI qualification with isolated protected homes.
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import net from 'node:net';
import { spawn, spawnSync, type ChildProcess } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { parseArgs } from 'node:util';
import { chromium, expect } from '@playwright/test';
import argon2 from 'argon2';
import * as yaml from 'js-yaml';

const { values } = parseArgs({ options: {
  output: { type: 'string' }, peer: { type: 'string' },
  'local-bind': { type: 'string' }, 'remote-bind': { type: 'string' },
  'local-interface': { type: 'string' }, 'remote-interface': { type: 'string' },
} });
const output = values.output, peer = values.peer;
const localBind = values['local-bind'], remoteBind = values['remote-bind'];
if (!output || !path.isAbsolute(output) || !peer || !/^[a-zA-Z0-9][a-zA-Z0-9._-]*$/.test(peer)
  || !localBind || !remoteBind || [localBind, remoteBind].some(ip => net.isIP(ip) !== 4 || ip.startsWith('127.') || ip === '0.0.0.0')) {
  throw new Error('Require --output FRESH_ABSOLUTE_DIRECTORY --peer AUTHORIZED_LINUX_ARM64_SSH_HOST --local-bind MACOS_LAN_IP --remote-bind LINUX_LAN_IP');
}
if (process.platform !== 'darwin' || process.arch !== 'arm64') throw new Error('Native macOS ARM64 primary required');
process.umask(0o077);
fs.mkdirSync(output, { mode: 0o700 });
const root = path.resolve(import.meta.dirname, '../..');
const quote = (s: string) => `'${s.replaceAll("'", "'\\''")}'`;
const sshArgs = ['-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes', '-o', 'ConnectTimeout=5'];
const sha = (data: string | Buffer) => createHash('sha256').update(data).digest('hex');
function run(program: string, args: string[], input?: string, cwd = root, timeout = 120_000): string {
  const r = spawnSync(program, args, { cwd, input, encoding: 'utf8', timeout, maxBuffer: 8 * 1024 * 1024 });
  if (r.error) throw r.error;
  if (r.status !== 0) throw new Error(`${program} exited ${r.status}: ${r.stderr.slice(-2000)}`);
  return r.stdout.trim();
}
const remote = (code: string, input?: unknown) => run('ssh', [...sshArgs, peer!, 'python3 -c '+quote(code)], input === undefined ? undefined : JSON.stringify(input), root, 20_000);
function inventory() {
  return Object.fromEntries(run('git', ['ls-files', '-z']).split('\0').filter(Boolean).map(name => {
    const full = path.join(root, name);
    return [name, sha(fs.lstatSync(full).isSymbolicLink() ? 'symlink:'+fs.readlinkSync(full) : fs.readFileSync(full))];
  }));
}
const revision = run('git', ['rev-parse', 'HEAD']);
if (run('git', ['status', '--porcelain'])) throw new Error('Commit the qualifier before native qualification');
const sourcesBefore = inventory();
const checks: string[] = [];
const receipt: Record<string, unknown> = { passed: false, revision, sourcesBefore, checks,
  scope: 'Actual protected cmd/webmux processes and browser enrollment; isolated homes and process-local Linux fixture certificate trust. Not installed-service, OS trust or real-agent/MAC acceptance.' };
const passed = (name: string) => { checks.push(name); console.log(`PASS ${name}`); };
function child(program: string, args: string[], log: string, env = process.env) {
  const fd = fs.openSync(path.join(output!, log), 'a', 0o600);
  try {
    const p = spawn(program, args, { env, stdio: ['pipe', fd, fd] });
    p.on('error', error => { receipt.spawnError = error.message; });
    return p;
  } finally { fs.closeSync(fd); }
}
async function stop(p: ChildProcess | undefined) {
  if (!p?.pid || p.exitCode !== null || p.signalCode !== null) return;
  const ended = new Promise<void>(resolve => p.once('exit', () => resolve()));
  p.kill('SIGTERM');
  const timer = setTimeout(() => p.kill('SIGKILL'), 5000);
  try { await ended; } finally { clearTimeout(timer); }
}
async function freePort(bind: string) {
  const s = net.createServer();
  await new Promise<void>((resolve, reject) => { s.once('error', reject); s.listen(0, bind, resolve); });
  const port = (s.address() as net.AddressInfo).port;
  await new Promise<void>((resolve, reject) => s.close(error => error ? reject(error) : resolve()));
  return port;
}
type Directory = { servers: Array<{ id: string; url: string; identity?: { node_id: string; owner: string; tls_key_sha256: string } }> };
type Candidate = { claimed_id: string; host: string; address: string; port: number; tls: boolean };
const localHome = path.join(output, 'receiver-home');
let remoteRoot = '', localProcess: ChildProcess | undefined, remoteProcess: ChildProcess | undefined, tunnel: ChildProcess | undefined;
let browser: Awaited<ReturnType<typeof chromium.launch>> | undefined;
let token = '', controllerURL = '';
const localEnv: NodeJS.ProcessEnv = Object.fromEntries(['PATH','HOME','TMPDIR','LANG','TZ'].flatMap(k => process.env[k] ? [[k, process.env[k]]] : []));
const launcher = `import os,sys,json,pathlib,subprocess,hashlib
p=json.load(sys.stdin);r=pathlib.Path(p['root']);b=r/'webmux'
assert hashlib.sha256(b.read_bytes()).hexdigest()==p['sha256']
h=r/'home';(h/'config').mkdir(parents=True,exist_ok=True)
for n,v in p['files'].items():
 f=h/'config'/n
 if not f.exists():f.write_text(v);f.chmod(0o600)
env={k:v for k,v in os.environ.items() if k in ('PATH','HOME','TMPDIR','LANG','TZ')}
env.update(p['env']);env.update(WEBMUX_HOME=str(h),WEBMUX_ROOT='',SSL_CERT_FILE=str(h/'config'/'fixture-ca.pem'))
with (r/'server.log').open('ab') as log:
 child=subprocess.Popen([str(b)],env=env,stdin=subprocess.DEVNULL,stdout=log,stderr=subprocess.STDOUT)
 (r/'pid').write_text(str(child.pid));sys.exit(child.wait())`;
const tlsDiagnostic = String.raw`package main
import("context";"crypto/tls";"encoding/json";"net";"os";"runtime";"time")
func probe(host,address string) string {
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel()
 d:=tls.Dialer{NetDialer:&net.Dialer{},Config:&tls.Config{MinVersion:tls.VersionTLS12,ServerName:host,NextProtos:[]string{"http/1.1"}}}
 c,e:=d.DialContext(ctx,"tcp",address);if e!=nil{return e.Error()};c.Close();return "ok"
}
func main(){
 host,ip,port:=os.Args[1],os.Args[2],os.Args[3]
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel()
 addresses,e:=net.DefaultResolver.LookupIPAddr(ctx,host)
 result:=map[string]any{"os":runtime.GOOS,"arch":runtime.GOARCH,"lookup_addresses":addresses,"hostname_tls":probe(host,net.JoinHostPort(host,port)),"direct_address_tls":probe(host,net.JoinHostPort(ip,port))}
 if e!=nil{result["lookup_error"]=e.Error()};json.NewEncoder(os.Stdout).Encode(result)
}
`;
async function stopRemote() {
  if (!remoteProcess || !remoteRoot) return;
  remote(`import os,sys,json,pathlib,signal,time
r=pathlib.Path(json.load(sys.stdin)['root']);pid=int((r/'pid').read_text());exe=r/'webmux'
for sig in (signal.SIGTERM,signal.SIGKILL):
 try:
  if (pathlib.Path('/proc')/str(pid)/'exe').resolve(strict=True)!=exe:raise RuntimeError('owned executable mismatch')
  os.kill(pid,sig)
 except (ProcessLookupError,FileNotFoundError):break
 for _ in range(50):
  if not (pathlib.Path('/proc')/str(pid)/'exe').exists():break
  time.sleep(.1)
 else:continue
 break`, { root: remoteRoot });
  await stop(remoteProcess);
  remoteProcess = undefined;
}
async function api(endpoint: string, method = 'GET', body?: unknown) {
  return fetch(controllerURL+'/api'+endpoint, { method, headers: { Authorization: 'Bearer '+token, 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(5000) });
}
async function directory() {
  const r = await api('/server-directory'); expect(r.status).toBe(200); return await r.json() as Directory;
}
try {
  const native = JSON.parse(remote(`import os,json;print(json.dumps({'kernel':os.uname().sysname,'machine':os.uname().machine}))`));
  expect(native).toEqual({ kernel: 'Linux', machine: 'aarch64' });
  receipt.remoteRuntime = native;
  run('npm', ['run', 'build', '--workspace=frontend'], undefined, path.join(root, 'webmux'));
  for (const target of ['darwin', 'linux']) run('go', ['run', './cmd/build', '-goos', target, '-goarch', 'arm64', '-o', path.join(output, target+'-webmux')], undefined, path.join(root, 'webmux/server'));
  receipt.binarySHA256 = Object.fromEntries(['darwin','linux'].map(target => [target,sha(fs.readFileSync(path.join(output,target+'-webmux')))]));
  fs.writeFileSync(path.join(output,'tls-diagnostic.go'),tlsDiagnostic);
  const diagnosticBuild=spawnSync('go',['build','-trimpath','-o',path.join(output,'tls-diagnostic'),path.join(output,'tls-diagnostic.go')],{env:{...process.env,GOOS:'linux',GOARCH:'arm64',CGO_ENABLED:'0'},encoding:'utf8',timeout:120_000});
  fs.writeFileSync(path.join(output,'tls-diagnostic-build.log'),diagnosticBuild.stdout+diagnosticBuild.stderr);
  if(diagnosticBuild.error)throw diagnosticBuild.error;
  if(diagnosticBuild.status!==0)throw new Error('Native TLS diagnostic build failed; retain private log');
  const diagnosticHash=sha(fs.readFileSync(path.join(output,'tls-diagnostic')));receipt.diagnosticSHA256=diagnosticHash;
  const prepared = JSON.parse(remote(`import os,tempfile,json,pathlib,socket
base=pathlib.Path.home()/'.cache'/'webmux';base.mkdir(exist_ok=True)
r=tempfile.mkdtemp(prefix='discovered-app-',dir=base);os.chmod(r,0o700)
s=socket.socket();s.bind((${JSON.stringify(remoteBind)},0));port=s.getsockname()[1];s.close()
print(json.dumps({'root':r,'port':port}))`)) as { root: string; port: number };
  remoteRoot = prepared.root;
  if (!/^\/[A-Za-z0-9_./-]+\/discovered-app-[A-Za-z0-9_]+$/.test(remoteRoot)) throw new Error('Unexpected owned remote path');
  receipt.remoteRoot = remoteRoot;
  run('scp', ['-B', ...sshArgs, path.join(output, 'linux-webmux'), `${peer}:${remoteRoot}/webmux`]);
  run('scp', ['-B', ...sshArgs, path.join(output, 'tls-diagnostic'), `${peer}:${remoteRoot}/tls-diagnostic`]);
  const httpPort = await freePort(localBind), tlsPort = await freePort(localBind), movedPort = await freePort(localBind), forward = await freePort('127.0.0.1');
  const password = randomBytes(24).toString('hex'), grant = randomBytes(32).toString('hex');
  const nodeID = randomBytes(16).toString('hex'), controllerID = randomBytes(16).toString('hex');
  const auth = yaml.dump({ auth: { mode: 'local', users: [{ username: 'qualifier', password_hash: await argon2.hash(password), admin: true }] } });
  const defaults = Object.fromEntries(fs.readdirSync(path.join(root, 'webmux/config.defaults'), { withFileTypes: true }).filter(e => e.isFile()).map(e => [e.name,fs.readFileSync(path.join(root,'webmux/config.defaults',e.name),'utf8')]));
  const app = (bind: string, http: number, tls: number) => {
    const data = yaml.load(defaults['app.yaml']) as { app: Record<string, unknown> };
    Object.assign(data.app, { listen_host: bind, http_port: http, https_port: tls }); return yaml.dump(data);
  };
  fs.mkdirSync(path.join(localHome,'config/tls'), { recursive: true, mode: 0o700 });
  const hostname = os.hostname().replace(/\.local$/, '')+'.local';
  const certConfig = path.join(output,'certificate.cnf');
  fs.writeFileSync(certConfig, `[req]\nprompt=no\ndistinguished_name=dn\nx509_extensions=extensions\n[dn]\nCN=WebMux isolated native application\n[extensions]\nbasicConstraints=critical,CA:TRUE\nkeyUsage=critical,digitalSignature,keyCertSign\nextendedKeyUsage=serverAuth\nsubjectAltName=IP:${localBind},DNS:${hostname}\n`);
  run('openssl', ['req','-x509','-newkey','rsa:2048','-nodes','-days','1','-config',certConfig,'-keyout',path.join(localHome,'config/tls/key.pem'),'-out',path.join(localHome,'config/tls/cert.pem')]);
  const receiverFiles = { ...defaults, 'auth.yaml': auth, 'app.yaml': app(localBind,httpPort,tlsPort), 'macos-setup.yaml':'skipped: true\n',
    'node-discovery.yaml':yaml.dump({version:1,id:nodeID,browse:false,advertise:true,interfaces:values['local-interface']?[values['local-interface']]:[]}),
    'servers.yaml':yaml.dump({version:1,servers:[],grants:[{id:'native-application',owner:'qualifier',token_env:'NATIVE_APP_GRANT'}]}) };
  for (const [name,text] of Object.entries(receiverFiles)) fs.writeFileSync(path.join(localHome,'config',name),text,{mode:0o600});
  Object.assign(localEnv,{WEBMUX_HOME:localHome,WEBMUX_ROOT:'',NATIVE_APP_GRANT:grant});
  const payload = { root:remoteRoot,sha256:sha(fs.readFileSync(path.join(output,'linux-webmux'))),files:{...defaults,'auth.yaml':auth,'app.yaml':app(remoteBind,prepared.port,8443),'fixture-ca.pem':fs.readFileSync(path.join(localHome,'config/tls/cert.pem'),'utf8'),
    'node-discovery.yaml':yaml.dump({version:1,id:controllerID,browse:true,advertise:false,interfaces:values['remote-interface']?[values['remote-interface']]:[]}),
    'servers.yaml':yaml.dump({version:1,servers:[],grants:[]})},env:{} };
  const launchRemote = () => { const p=child('ssh',[...sshArgs,peer,'python3 -c '+quote(launcher)],'controller-ssh.log');p.stdin!.end(JSON.stringify(payload));return p; };
  localProcess=child(path.join(output,'darwin-webmux'),[],'receiver.log',localEnv);
  remoteProcess=launchRemote();
  controllerURL=`http://127.0.0.1:${forward}`;
  tunnel=child('ssh',[...sshArgs,'-o','ExitOnForwardFailure=yes','-N','-L',`127.0.0.1:${forward}:${remoteBind}:${prepared.port}`,peer],'tunnel.log');
  await expect.poll(async()=>{try{return(await fetch(controllerURL+'/api/health',{signal:AbortSignal.timeout(1000)})).status;}catch{return 0;}},{timeout:20_000}).toBe(200);
  const login=await fetch(controllerURL+'/api/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:'qualifier',password})});
  expect(login.status).toBe(200);token=(await login.json() as {token:string}).token;
  expect((await fetch(controllerURL+'/api/discovered-nodes')).status).toBe(401);
  let observed: Candidate | undefined;
  await expect.poll(async()=>{const r=await api('/discovered-nodes');expect(r.status).toBe(200);const view=await r.json() as {candidates:Candidate[]};observed=view.candidates.find(c=>c.claimed_id===nodeID&&c.port===tlsPort&&c.address===localBind&&c.tls);return !!observed;},{timeout:8000,intervals:[100,200,500]}).toBe(true);
  expect((await directory()).servers).toEqual([]);
  passed('actual Linux application discovers macOS HTTPS application without enrolling it');
  const diagnostic=remote(`import os,sys,json,pathlib,subprocess,hashlib
p=json.load(sys.stdin);r=pathlib.Path(p['root']);b=r/'tls-diagnostic';assert hashlib.sha256(b.read_bytes()).hexdigest()==p['sha256']
env=dict(os.environ,SSL_CERT_FILE=str(r/'home'/'config'/'fixture-ca.pem'))
result=subprocess.run([str(b),p['host'],p['address'],str(p['port'])],env=env,capture_output=True,text=True,timeout=12)
sys.stdout.write(result.stdout);sys.stderr.write(result.stderr);sys.exit(result.returncode)`,{root:remoteRoot,sha256:diagnosticHash,host:observed!.host,address:localBind,port:tlsPort});
  fs.writeFileSync(path.join(output,'transport-diagnostic.json'),diagnostic+'\n');receipt.transportDiagnostic=JSON.parse(diagnostic);
  browser=await chromium.launch({headless:true,...(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH?{executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH}:{})});
  const context=await browser.newContext({viewport:{width:1440,height:1000}});
  await context.addInitScript(token=>localStorage.setItem('webmux_token',token),token);
  const page=await context.newPage();await page.goto(controllerURL);
  await page.getByRole('group',{name:'WebMux cluster'}).getByRole('button',{name:'Manage servers',exact:true}).click();
  const manager=page.getByRole('dialog',{name:'Manage servers'});
  const candidate=manager.locator('section').filter({hasText:nodeID}).filter({hasText:`${localBind}:${tlsPort}`});
  await expect(candidate).toHaveCount(1);
  await candidate.getByRole('button',{name:`Configure connection to ${observed!.host}`,exact:true}).click();
  await expect(manager.getByLabel('Server URL',{exact:true})).toHaveValue(`https://${observed!.host}:${tlsPort}`);
  expect((await directory()).servers).toEqual([]);
  await manager.getByLabel('Name (optional)',{exact:true}).fill('Native discovered peer');
  await manager.getByLabel('Credential',{exact:true}).fill(grant);
  await manager.getByRole('button',{name:'Save server',exact:true}).click();
  await expect(manager.getByRole('status',{name:'Server directory status'})).toContainText('Saved');
  const saved=await directory();expect(saved.servers).toHaveLength(1);
  const peerEntry=saved.servers[0];expect(peerEntry.identity?.node_id).toBeFalsy();
  await manager.getByRole('button',{name:'Verify identity of Native discovered peer',exact:true}).click();
  await expect(manager.getByRole('status',{name:'Server directory status'})).toContainText('identity verified',{timeout:10_000});
  const enrolled=(await directory()).servers[0];expect(enrolled.identity).toMatchObject({node_id:nodeID,owner:'qualifier'});
  expect((await api(`/servers/${enrolled.id}/sessions`)).status).toBe(200);
  passed('human browser configures the discovered HTTPS endpoint and explicitly verifies node/account identity');
  await page.screenshot({path:path.join(output,'enrolled.png'),fullPage:true});await browser.close();browser=undefined;
  await stop(localProcess);localProcess=undefined;
  fs.writeFileSync(path.join(localHome,'config/app.yaml'),app(localBind,httpPort,movedPort));
  localProcess=child(path.join(output,'darwin-webmux'),[],'receiver.log',localEnv);
  await expect.poll(async()=>{const entry=(await directory()).servers.find(s=>s.id===enrolled.id);return entry?.url.endsWith(':'+movedPort);},{timeout:20_000,intervals:[200,500,1000]}).toBe(true);
  const migrated=(await directory()).servers[0];expect(migrated.identity).toEqual(enrolled.identity);
  expect((await api(`/servers/${enrolled.id}/sessions`)).status).toBe(200);
  passed('actual receiver restart and changed port migrate in the controller background without an open browser');
  await stopRemote();remoteProcess=launchRemote();
  await expect.poll(async()=>{try{return(await api(`/servers/${enrolled.id}/sessions`)).status;}catch{return 0;}},{timeout:20_000}).toBe(200);
  expect((await directory()).servers[0]).toMatchObject({id:enrolled.id,url:migrated.url,identity:enrolled.identity});
  passed('actual controller restart retains verified peer identity and migrated endpoint');
  receipt.passed=true;
} catch(error) {
  receipt.error=error instanceof Error?error.message:String(error);
  if(browser)try{await browser.contexts()[0]?.pages()[0]?.screenshot({path:path.join(output,'failure.png'),fullPage:true});}catch{receipt.failureScreenshotUnavailable=true;}
  throw error;
} finally {
  const cleanup=await Promise.allSettled([browser?.close(),stop(localProcess),stopRemote()]);
  receipt.cleanup=cleanup.map(r=>r.status==='fulfilled'?'completed':String(r.reason));
  await stop(tunnel);
  if(remoteRoot)try{fs.writeFileSync(path.join(output,'controller.log'),remote(`import pathlib,sys,json;print((pathlib.Path(json.load(sys.stdin)['root'])/'server.log').read_text())`,{root:remoteRoot}));}catch{receipt.remoteLogUnavailable=true;}
  const sourcesAfter=inventory();receipt.sourcesUnchanged=JSON.stringify(sourcesBefore)===JSON.stringify(sourcesAfter);
  if(!receipt.sourcesUnchanged||cleanup.some(r=>r.status==='rejected'))receipt.passed=false;
  fs.writeFileSync(path.join(output,'receipt.json'),JSON.stringify(receipt,null,2)+'\n',{mode:0o600});
}
if(!receipt.passed)process.exitCode=1;
