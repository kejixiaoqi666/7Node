#!/usr/bin/env python3
"""Build in a fresh private directory; never patch shared Go module caches.

Pinned modules are verified by Go's checksum database and exact per-file hashes.
The generated tree and its dependency sources remain available for reproduction.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess

def run(*args, cwd=None, env=None):
    subprocess.run(args, cwd=cwd, env=env, check=True)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--work', type=Path, required=True)
    parser.add_argument('--go', default='go')
    parser.add_argument('--test-race', action='store_true')
    args = parser.parse_args()
    source = Path(__file__).resolve().parent
    work = args.work.resolve()
    work.mkdir(parents=True, exist_ok=False)
    env = dict(os.environ, GOTOOLCHAIN='go1.26.8', GOWORK='off', CGO_ENABLED='0')
    project = work/'project'
    project.mkdir()
    for p in source.iterdir():
        if p.is_file() and (p.suffix == '.go' or p.name in ('go.mod','go.sum')):
            shutil.copyfile(p, project/p.name)
    manifest = json.loads((source/'patches/manifest.json').read_text())
    dep_paths = {}
    for name, module in manifest['modules'].items():
        result = subprocess.run([args.go, 'mod', 'download', '-json', module], cwd=project, env=env, capture_output=True, text=True, check=True)
        result = json.loads(result.stdout)
        cache = Path(result['Dir'])
        dep = work/'dependencies'/name
        dep.parent.mkdir(exist_ok=True)
        shutil.copytree(cache, dep)
        dep_paths[name] = dep
    for patch in manifest['files']:
        name, rel = patch['path'].split('/',1)
        target = dep_paths[name]/rel
        replacement = source/'patches'/patch['path']
        if hashlib.sha256(target.read_bytes()).hexdigest() != patch['upstream_sha256']:
            raise RuntimeError('unexpected upstream source: '+patch['path'])
        if hashlib.sha256(replacement.read_bytes()).hexdigest() != patch['patched_sha256']:
            raise RuntimeError('unexpected replacement source: '+patch['path'])
        target.chmod(0o644)
        shutil.copyfile(replacement,target)
    for upstream, name in [('github.com/sagernet/sing-box','singbox'),('github.com/sagernet/sing-vmess','singvmess')]:
        run(args.go,'mod','edit','-replace='+upstream+'='+str(dep_paths[name]),cwd=project,env=env)
    run(args.go,'mod','tidy',cwd=project,env=env)
    run(args.go,'test','./...',cwd=project,env=env)
    if args.test_race:
        run(args.go,'test','-race','./...',cwd=project,env=dict(env,CGO_ENABLED='1'))
    output = work/'xbord-native-users'
    run(args.go,'build','-trimpath','-ldflags=-s -w','-o',str(output),'.',cwd=project,env=env)
    print(json.dumps({'binary':str(output),'bytes':output.stat().st_size,'sha256':hashlib.sha256(output.read_bytes()).hexdigest(),'work':str(work)}))

if __name__ == '__main__':
    main()
