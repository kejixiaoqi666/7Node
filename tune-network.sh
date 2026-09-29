#!/usr/bin/env bash
# High-throughput TCP profile. No kernel replacement, firewall or tc deletion.
set -Eeuo pipefail
[[ ${EUID} == 0 ]] || { echo '请以 root 执行'; exit 1; }
command -v python3 >/dev/null || { echo '请先安装 python3'; exit 1; }
command -v sysctl >/dev/null || { echo '请先安装 procps'; exit 1; }
exec python3 - "${1:---apply}" <<'PY'
import fcntl, json, os, pathlib, shutil, subprocess, sys, time

os.umask(0o077)
state=pathlib.Path('/var/lib/xboard-net-tune')
config=pathlib.Path('/etc/sysctl.d/99-zz-xboard-performance.conf')
keys=['net.ipv4.tcp_congestion_control','net.core.default_qdisc',
      'net.core.rmem_max','net.core.wmem_max','net.ipv4.tcp_rmem',
      'net.ipv4.tcp_wmem','net.ipv4.tcp_moderate_rcvbuf',
      'net.ipv4.tcp_window_scaling','net.ipv4.tcp_sack','net.ipv4.tcp_mtu_probing']
def run(args):
    return subprocess.run(args,check=True,text=True,capture_output=True).stdout.strip()
def read(key): return run(['sysctl','-n',key])
def write(values):
    for k,v in values.items(): run(['sysctl','-w',k+'='+v])
def save_file(text):
    if text is None:
        config.unlink(missing_ok=True)
    else:
        tmp=config.with_suffix('.tmp')
        tmp.write_text(text);tmp.chmod(0o644);tmp.replace(config)
def status():
    for k in keys: print(k+' = '+read(k))
    if shutil.which('tc'): print(run(['tc','qdisc','show']))

mode=sys.argv[1]
if mode not in ('--apply','--status','--rollback'):
    sys.exit('用法：bash tune-network.sh [--apply|--status|--rollback]')
if mode=='--status':
    status();sys.exit(0)
state.mkdir(mode=0o700,parents=True,exist_ok=True)
lock=(state/'lock').open('w');fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
backup=state/'original.json'
if mode=='--rollback':
    if not backup.exists():sys.exit('没有本脚本的恢复备份')
    original=json.loads(backup.read_text())
    write(original['values']);save_file(original['file'])
    backup.rename(state/('restored-'+str(time.time_ns())+'.json'))
    print('已恢复首次执行前的参数和配置文件。');status();sys.exit(0)

if shutil.which('modprobe'):
    subprocess.run(['modprobe','tcp_bbr'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    subprocess.run(['modprobe','sch_fq'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
if 'bbr' not in read('net.ipv4.tcp_available_congestion_control').split():
    sys.exit('当前内核不提供 BBR；未修改配置。请使用发行版支持 BBR 的内核。')
memory=int(next(x.split()[1] for x in pathlib.Path('/proc/meminfo').read_text().splitlines() if x.startswith('MemTotal:')))
# Per-socket maximum, not a preallocation. Leave global tcp_mem under kernel control.
ceiling=64*1024*1024 if memory>=768*1024 else 32*1024*1024
if memory<384*1024:sys.exit('内存小于 384 MiB，不适合此高吞吐配置；未修改配置。')
before={k:read(k) for k in keys}
old_file=config.read_text() if config.exists() else None
values=dict(before)
values.update({'net.ipv4.tcp_congestion_control':'bbr','net.core.default_qdisc':'fq',
               'net.ipv4.tcp_moderate_rcvbuf':'1','net.ipv4.tcp_window_scaling':'1',
               'net.ipv4.tcp_sack':'1'})
for k in ('net.core.rmem_max','net.core.wmem_max'):
    values[k]=str(max(int(before[k]),ceiling))
for k in ('net.ipv4.tcp_rmem','net.ipv4.tcp_wmem'):
    a,b,c=map(int,before[k].split());values[k]=f'{a} {b} {max(c,ceiling)}'
values['net.ipv4.tcp_mtu_probing']=str(max(1,int(before['net.ipv4.tcp_mtu_probing'])))
if not backup.exists():backup.write_text(json.dumps({'values':before,'file':old_file}))
try:
    write(values)
    config.parent.mkdir(parents=True,exist_ok=True)
    save_file('# Managed by xbord-node-v3/tune-network.sh; use --rollback to restore.\n'+
              '\n'.join(k+' = '+v for k,v in values.items())+'\n')
    for k,v in values.items():
        if read(k).split()!=v.split():raise RuntimeError('参数未生效：'+k)
except Exception:
    write(before);save_file(old_file)
    raise
print(f'已应用高吞吐配置；缓冲上限目标 {ceiling//1024//1024} MiB，BBR 已启用。')
print('新 TCP 连接使用新默认拥塞算法，请重新发起下载；无需重启 VPS 或节点。')
print('fq 已设为默认队列；现有网卡队列和运营商限速规则保持原样，不保证立即变为 fq。')
print('不突破线路/套餐限速，不修改账户限速，不保证千兆。BBR 不控制 Hysteria2 的 QUIC 拥塞算法。')
print('恢复命令：bash tune-network.sh --rollback')
status()
PY
