import pathlib,subprocess,tempfile,json,os
repo=pathlib.Path(__file__).resolve().parents[1]
s=(repo/'tune-network.sh').read_text();subprocess.run(['bash','-n'],input=s,text=True,check=True)
code=s.split("<<'PY'\n",1)[1].rsplit('\nPY',1)[0]
with tempfile.TemporaryDirectory() as td:
 p=pathlib.Path(td);(p/'bin').mkdir();(p/'proc').mkdir();(p/'etc/sysctl.d').mkdir(parents=True)
 (p/'proc/meminfo').write_text('MemTotal: 990000 kB\n')
 for x in ['/var/','/etc/','/proc/']:code=code.replace(x,str(p)+x)
 initial={'net.ipv4.tcp_available_congestion_control':'reno cubic bbr','net.ipv4.tcp_congestion_control':'cubic','net.core.default_qdisc':'fq_codel','net.core.rmem_max':'16777216','net.core.wmem_max':'16777216','net.ipv4.tcp_rmem':'4096 65536 16777216','net.ipv4.tcp_wmem':'4096 65536 16777216','net.ipv4.tcp_moderate_rcvbuf':'1','net.ipv4.tcp_window_scaling':'1','net.ipv4.tcp_sack':'1','net.ipv4.tcp_mtu_probing':'0'}
 db=p/'db.json';db.write_text(json.dumps(initial))
 f=p/'bin/sysctl';f.write_text('#!/usr/bin/env python3\nimport json,sys,pathlib\np=pathlib.Path('+repr(str(db))+')\nd=json.loads(p.read_text())\nif sys.argv[1]=="-n":print(d[sys.argv[2]])\nelse:\n k,v=sys.argv[2].split("=",1);d[k]=v;p.write_text(json.dumps(d))\n');f.chmod(0o755)
 for name in ['modprobe','tc']:
  f=p/'bin'/name;f.write_text('#!/bin/sh\nexit 0\n');f.chmod(0o755)
 env={**os.environ,'PATH':str(p/'bin')+':'+os.environ['PATH']}
 for mode in ['--apply','--apply','--rollback']:
  r=subprocess.run(['python3','-',mode],input=code,text=True,env=env,capture_output=True);assert r.returncode==0,r.stderr
 assert json.loads(db.read_text())==initial
 assert not (p/'etc/sysctl.d/99-zz-xboard-performance.conf').exists()
 initial['net.ipv4.tcp_available_congestion_control']='reno cubic';db.write_text(json.dumps(initial))
 r=subprocess.run(['python3','-','--apply'],input=code,text=True,env=env,capture_output=True);assert r.returncode!=0
 assert json.loads(db.read_text())==initial
 print('PASS: apply, repeated apply, exact rollback, unavailable BBR protection')
