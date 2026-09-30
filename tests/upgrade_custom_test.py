import pathlib,tempfile,subprocess,os,gzip,hashlib
source=(pathlib.Path(__file__).resolve().parents[1]/'upgrade-custom.sh').read_text()
for case in ['success','bad-hash','restart-failure']:
 with tempfile.TemporaryDirectory(prefix='upgrade-fixture-') as temp:
  root=pathlib.Path(temp);app=root/'app';conf=root/'conf';backups=root/'backups';fake=root/'bin'
  for p in [app,conf,backups,fake]:p.mkdir()
  (app/'xboard-node').write_bytes(b'old-binary');(conf/'config.yml').write_text('preserved')
  artifact=root/'release.gz';artifact.write_bytes(gzip.compress(b'new-binary',mtime=0))
  def command(name,body):
   p=fake/name;p.write_text('#!/bin/sh\n'+body+'\n');p.chmod(0o755)
  command('curl','while [ "$#" -gt 0 ]; do if [ "$1" = -o ]; then shift; cp "$FIXTURE_ARCHIVE" "$1"; exit; fi; shift; done; exit 1')
  command('sleep','exit 0')
  command('systemctl','if [ "$1" = restart ] && [ "$FIXTURE_CASE" = restart-failure ] && [ ! -f "$FIXTURE_ROOT/failed-once" ]; then touch "$FIXTURE_ROOT/failed-once"; exit 1; fi; exit 0')
  script=source.replace('/opt/xboard-node-custom',str(app)).replace('/etc/xboard-node-custom',str(conf)).replace('/root/xboard-node-upgrade-',str(backups/'upgrade-'))
  script=script.replace('0cffb212f033b54eca40aee9b5f0f15cee728af83636cf90d624b81ec282e45d',hashlib.sha256(artifact.read_bytes()).hexdigest() if case!='bad-hash' else '0'*64)
  script=script.replace('58ce7b14e5e2d23b6cd2765c2ac1771b1f247e4e7e2df1ea8e61a0d1bcbcdcda',hashlib.sha256(b'new-binary').hexdigest())
  p=root/'upgrade.sh';p.write_text(script)
  env=dict(os.environ,PATH=str(fake)+':'+os.environ['PATH'],FIXTURE_ROOT=str(root),FIXTURE_ARCHIVE=str(artifact),FIXTURE_CASE=case)
  r=subprocess.run(['bash',str(p)],env=env,capture_output=True,text=True)
  assert (r.returncode==0)==(case=='success'),(case,r.stdout,r.stderr)
  assert (app/'xboard-node').read_bytes()==(b'new-binary' if case=='success' else b'old-binary')
  assert (conf/'config.yml').read_text()=='preserved'
  assert any(x.read_bytes()==b'old-binary' for x in backups.glob('*/xboard-node'))
  print(case,'PASS')

