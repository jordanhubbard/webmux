#!/usr/bin/env python3
"""Run Linux native browser qualification with a disposable encrypted keyring."""
import os,sys,subprocess,tempfile,pathlib,time,secrets,json
os.umask(0o077)
if sys.platform != 'linux': raise SystemExit('Disposable Secret Service qualification requires Linux')
if len(sys.argv)<2:raise SystemExit('Usage: with-test-keyring.py COMMAND [ARGS...]')
if sys.argv[1]!='--inside':
 root=pathlib.Path(tempfile.mkdtemp(prefix='wk27.',dir='/tmp'))
 env=dict(os.environ,WEBMUX_TEST_KEYRING_ROOT=str(root),XDG_DATA_HOME=str(root/'data'),GNOME_KEYRING_CONTROL=str(root/'control'))
 env.pop('DBUS_SESSION_BUS_ADDRESS',None)
 r=subprocess.run(['dbus-run-session','--',sys.executable,__file__,'--inside',*sys.argv[1:]],env=env)
 raise SystemExit(r.returncode)
root=pathlib.Path(os.environ['WEBMUX_TEST_KEYRING_ROOT']);(root/'control').mkdir(mode=0o700);(root/'data').mkdir(mode=0o700)
log=(root/'keyring.log').open('wb')
daemon=subprocess.Popen(['gnome-keyring-daemon','--foreground','--unlock','--components=secrets','--control-directory='+str(root/'control')],stdin=subprocess.PIPE,stdout=log,stderr=log)
daemon.stdin.write((secrets.token_urlsafe(48)+'\n').encode());daemon.stdin.close()
try:
 ready=False;until=time.monotonic()+12
 while time.monotonic()<until:
  q=subprocess.run(['busctl','--user','--timeout=1','get-property','org.freedesktop.secrets','/org/freedesktop/secrets/collection/login','org.freedesktop.Secret.Collection','Locked'],capture_output=True,text=True,timeout=2)
  if q.returncode==0 and q.stdout.strip()=='b false':ready=True;break
  if daemon.poll() is not None:break
  time.sleep(.1)
 if not ready:raise RuntimeError('Disposable encrypted test keyring did not unlock')
 (root/'receipt.json').write_text(json.dumps({'private_bus':True,'private_data_directory':True,'encrypted_login_keyring_unlocked':True,'native_keyring_changed':False}))
 print('Disposable encrypted test keyring ready; real keyring unchanged.',flush=True)
 r=subprocess.run(sys.argv[2:]);result=r.returncode
finally:
 daemon.terminate()
 try:daemon.wait(timeout=5)
 except subprocess.TimeoutExpired:daemon.kill();daemon.wait(timeout=5)
 log.close()
raise SystemExit(result)
