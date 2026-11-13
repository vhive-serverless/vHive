#!/usr/bin/env bash
# PREPARED ONLY. Supply reviewed published Step 2 and final consumer revisions.
# Uses vHive's installer/acceptance entrypoints; does not dispatch remote work.
set -euo pipefail
umask 022
# Keep Python assertion guards and committed imports active.
unset PYTHONOPTIMIZE PYTHONPATH PYTHONHOME
: "${VHIVE_CHECKOUT:?Set the exact final clean vHive checkout on the idle worker}"
: "${EXPECTED_VHIVE_REVISION:?Set the full final committed vHive SHA; not known yet}"
: "${EXPECTED_PUBLISHED_STEP2_REVISION:?Set the full reviewed and published corrected Step 2 SHA; no default}"
: "${RUN_DIR:?Set a NEW absolute private run directory outside every checkout}"
: "${GO_BIN:?Set the existing absolute Go 1.26.11 executable}"
readonly RUNTIME_REVISION="$EXPECTED_PUBLISHED_STEP2_REVISION"
readonly RUNTIME_REPOSITORY=https://github.com/vhive-serverless/firecracker-containerd.git
readonly RUNTIME_REF=refs/heads/firecracker-v1.18-containerdv2-vhive-integration
readonly SDK_REVISION=dd9db322432989b96cff7a38a64703d60b6cabf9
[[ "$EXPECTED_VHIVE_REVISION" =~ ^[0-9a-f]{40}$ ]]
[[ "$RUNTIME_REVISION" =~ ^[0-9a-f]{40}$ ]]
[[ "$RUN_DIR" = /* && "$GO_BIN" = /* && -x "$GO_BIN" ]]
VHIVE_CHECKOUT=$(realpath "$VHIVE_CHECKOUT")
RUN_DIR=$(realpath -m "$RUN_DIR")
GO_BIN=$(realpath "$GO_BIN")
cd "$VHIVE_CHECKOUT"
test "$(git rev-parse HEAD)" = "$EXPECTED_VHIVE_REVISION"
test -z "$(git status --porcelain)"
test "$("$GO_BIN" version)" = 'go version go1.26.11 linux/amd64'
python3 -c 'import sys,tomllib; assert sys.version_info >= (3,11)'
[[ "$RUN_DIR/" != "$VHIVE_CHECKOUT/"* && ! -e "$RUN_DIR" ]]
# Cooperative lock is additional to the mandatory worker-idle precondition.
exec 9>/tmp/vhive-installed-consumer.lock
flock -n 9
mkdir -m 700 "$RUN_DIR"
readonly EVIDENCE="$RUN_DIR/evidence"
mkdir "$EVIDENCE"
# Reuse the exact successful installation from the capacity-failed attempt.
# The committed install command verifies its receipt and every artifact again.
export STACK_DIR=/users/nehalem/fc-ctrd-mod/vhive-installed-final-7efebc4b-isolated-20261113/stack
export INSTALL_DIR=/users/nehalem/fc-ctrd-mod/vhive-installed-final-7efebc4b-isolated-20261113/install
python3 - "$INSTALL_DIR/vhive-stack-receipt.json" "$EVIDENCE" <<'RECEIPT'
import hashlib,pathlib,sys
source=pathlib.Path(sys.argv[1]); data=source.read_bytes()
assert hashlib.sha256(data).hexdigest()=='9cd10430db5acea75593df0193d5987698ee5990b7765a18bc7e0556ba05d1e4'
pathlib.Path(sys.argv[2],'reused-install-receipt.json').write_bytes(data)
RECEIPT
export FICD_DM_POOL="vhive-final-${EXPECTED_VHIVE_REVISION:0:12}-$$"
export FICD_DM_VOLUME_GROUP='' GOWORK=off GOTOOLCHAIN=local GOENV=off GOFLAGS=''
export PYTHONDONTWRITEBYTECODE=1
export PATH="$(dirname "$GO_BIN"):/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
unset VHIVE_RUNTIME_REVISION VHIVE_RUNTIME_CHECKOUT ALLOW_DIRTY GO111MODULE
readonly STATUS="$EVIDENCE/commands.tsv"
printf 'step\texit_code\n' > "$STATUS"
run_logged() {
    local label=$1 result
    shift
    test ! -e "$EVIDENCE/$label.log"
    if "$@" > "$EVIDENCE/$label.log" 2>&1; then
        printf '%s\t0\n' "$label" >> "$STATUS"
    else
        result=$?
        printf '%s\t%s\n' "$label" "$result" >> "$STATUS"
        return "$result"
    fi
}

# Record/restore only the ten files activate() actually changes. Never stop
# services, flush networks, reset shared pools, or delete existing state trees.
host_state() {
sudo -n python3 - "$1" "$EVIDENCE" "$VHIVE_CHECKOUT" "$INSTALL_DIR" "$FICD_DM_POOL" "${VHIVE_HISTORICAL_DAEMON_ALLOWLIST:?Set the reviewed exact worker-daemon allowlist JSON}" <<'PY'
import errno,hashlib,json,os,pathlib,re,shutil,socket,stat,subprocess,sys
mode,out,checkout,prefix,pool,allowlist_path=sys.argv[1:]
out=pathlib.Path(out); checkout=pathlib.Path(checkout)
configs=['/etc/firecracker-containerd/config.toml','/etc/containerd/firecracker-runtime.json','/etc/demux-snapshotter/config.toml']
bins=['firecracker','jailer','firecracker-containerd','firecracker-ctr','containerd-shim-aws-firecracker','demux-snapshotter','http-address-resolver']
paths=configs+['/usr/local/bin/'+name for name in bins]
roots=['/var/lib/firecracker-containerd/containerd','/var/lib/firecracker-containerd/runtime','/var/lib/firecracker-containerd/snapshotter/devmapper','/run/firecracker-containerd','/var/lib/demux-snapshotter','/fccd']
sockets=['/run/firecracker-containerd/containerd.sock','/run/firecracker-containerd/containerd.sock.ttrpc','/var/lib/demux-snapshotter/snapshotter.sock']
def identity(p):
    if not os.path.lexists(p): return {'kind':'absent'}
    s=os.lstat(p); meta={'mode':stat.S_IMODE(s.st_mode),'uid':s.st_uid,'gid':s.st_gid}
    if stat.S_ISLNK(s.st_mode): return dict(meta,kind='symlink',target=os.readlink(p))
    if stat.S_ISREG(s.st_mode): return dict(meta,kind='file',sha256=hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest())
    raise RuntimeError('Unsupported activation file: '+p)
def idle():
    # Preserve only the eight audited, root-owned, private-socket idle daemons.
    # Pin /proc/exe itself: one historical executable pathname is deleted.
    data=pathlib.Path(allowlist_path).read_bytes()
    if hashlib.sha256(data).hexdigest()!='710baa09cf50b9bb181e55b37f0f2b4b5d31c406379aacf14b860a4a362e575a':
        raise RuntimeError('Historical-daemon allowlist differs from reviewed bytes')
    allow=json.loads(data)
    if allow['schema']!=1 or pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()!=allow['boot_id']:
        raise RuntimeError('Historical-daemon allowlist is for another boot')
    expected={x['pid']:x for x in allow['allowed']}
    if len(expected)!=8 or len(expected)!=len(allow['allowed']):
        raise RuntimeError('Expected exactly eight distinct audited daemon identities')
    seen=set()
    for proc in pathlib.Path('/proc').glob('[0-9]*'):
        try: name=pathlib.Path(os.readlink(proc/'exe').removesuffix(' (deleted)')).name
        except (FileNotFoundError,ProcessLookupError): continue
        if name not in bins: continue
        wanted=expected.get(int(proc.name))
        if name!='firecracker-containerd' or wanted is None:
            raise RuntimeError('Unaudited Firecracker-related process: '+str(proc)+' '+name)
        try:
            exe=os.readlink(proc/'exe'); info=os.stat(proc/'exe')
            raw=(proc/'cmdline').read_bytes()
            process_stat=(proc/'stat').read_text()
            fields=process_stat[process_stat.rindex(')')+2:].split()
            actual={'pid':int(proc.name),'start_ticks':int(fields[19]),
                    'exe':exe,'exe_device':info.st_dev,'exe_inode':info.st_ino,
                    'exe_sha256':hashlib.sha256((proc/'exe').read_bytes()).hexdigest(),
                    'uid':next(x for x in (proc/'status').read_text().splitlines() if x.startswith('Uid:')).split()[1:],
                    'cwd':os.readlink(proc/'cwd'),'cgroup':(proc/'cgroup').read_text().strip(),
                    'argv':[x.decode(errors='replace') for x in raw.rstrip(b'\0').split(b'\0')],
                    'cmdline_sha256':hashlib.sha256(raw).hexdigest(),
                    'config_sha256':{p:hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest() for p in wanted['config_sha256']}}
        except (FileNotFoundError,ProcessLookupError) as error:
            raise RuntimeError('Audited historical daemon disappeared or changed: '+str(proc)) from error
        if actual!=wanted:
            raise RuntimeError('Audited historical daemon identity/config changed: '+str(proc))
        seen.add(int(proc.name))
    if seen!=set(expected):
        raise RuntimeError('Audited historical daemon missing/restarted: '+repr(sorted(set(expected)-seen)))
def networks():
    return sorted(p.name for p in pathlib.Path('/run/netns').glob('*') if re.fullmatch(r'uvmns[0-9]+',p.name))
def veths():
    links=json.loads(subprocess.check_output(['ip','-j','link']))
    return sorted(x['ifname'] for x in links if re.fullmatch(r'veth[0-9]+-[01]',x['ifname']))
if mode=='before':
    idle()
    for p in sockets:
        if os.path.lexists(p): raise RuntimeError('Existing endpoint; do not remove it: '+p)
    for p in roots:
        path=pathlib.Path(p)
        if path.is_symlink() or (path.exists() and (not path.is_dir() or any(path.iterdir()))):
            raise RuntimeError('Existing/nonempty runtime state; use an idle dedicated worker: '+p)
    if networks() or veths(): raise RuntimeError('Existing vHive namespaces/interfaces; do not delete them')
    old={p:identity(p) for p in paths}
    if any(old[p]['kind']=='symlink' for p in configs): raise RuntimeError('Configuration symlink requires manual review')
    backup=out/'activation-backup'; backup.mkdir()
    for i,p in enumerate(paths):
        if old[p]['kind']=='file': shutil.copy2(p,backup/str(i))
    rendered=(checkout/'configs/firecracker-containerd/config.toml').read_text().replace('fc-dev-thinpool',pool)
    runtime=json.loads((checkout/'configs/firecracker-containerd/firecracker-runtime.json').read_text())
    def render(v):
        if isinstance(v,dict): return {k:render(x) for k,x in v.items()}
        if isinstance(v,list): return [render(x) for x in v]
        return v.replace('${STACK_PREFIX}',prefix) if isinstance(v,str) else v
    contents=[rendered,json.dumps(render(runtime),indent=2)+'\n',(checkout/'configs/demux-snapshotter/config.toml').read_text()]
    expected={p:{'kind':'file','sha256':hashlib.sha256(s.encode()).hexdigest(),'mode':0o644,'uid':0,'gid':0} for p,s in zip(configs,contents)}
    expected.update({p:{'kind':'symlink','target':prefix+'/bin/'+name,'mode':0o777,'uid':0,'gid':0} for p,name in zip(paths[3:],bins)})
    state={'before':old,'expected':expected,'networks':networks(),'fccd_mode':stat.S_IMODE(os.stat('/fccd').st_mode) if os.path.exists('/fccd') else None}
    (out/'activation-state.json').write_text(json.dumps(state,indent=2)+'\n')
elif mode=='restore':
    idle()
    state=json.loads((out/'activation-state.json').read_text())
    if networks()!=state['networks'] or veths(): raise RuntimeError('Leaked/new vHive networking; retain evidence and inspect, never flush networks')
    for p in sockets:
        if not os.path.lexists(p): continue
        if not stat.S_ISSOCK(os.lstat(p).st_mode): raise RuntimeError('Unexpected endpoint type: '+p)
        with socket.socket(socket.AF_UNIX) as connection:
            error=connection.connect_ex(p)
        if error!=errno.ECONNREFUSED: raise RuntimeError('Endpoint still active/uncertain: '+p)
        os.unlink(p) # absent before this exclusively owned run, no daemon alive
    for i,p in enumerate(paths):
        current=identity(p); old=state['before'][p]
        if current==old: continue
        if current!=state['expected'][p]: raise RuntimeError('Activation changed unexpectedly; refuse overwrite: '+p)
        os.unlink(p)
        if old['kind']=='file':
            shutil.copy2(out/'activation-backup'/str(i),p); os.chown(p,old['uid'],old['gid'])
        elif old['kind']=='symlink':
            os.symlink(old['target'],p); os.lchown(p,old['uid'],old['gid'])
        assert identity(p)==old,p
    if state['fccd_mode'] is not None: os.chmod('/fccd',state['fccd_mode'])
    retained={p:sorted(x.name for x in pathlib.Path(p).iterdir()) for p in roots if pathlib.Path(p).is_dir()}
    (out/'retained-state.json').write_text(json.dumps(retained,indent=2)+'\n')
    (out/'activation-restored.json').write_text(json.dumps({p:identity(p) for p in paths},indent=2)+'\n')
else: raise RuntimeError('Unknown mode')
PY
}

pool_created=false
activation_started=false
cleanup() {
    local result=$? cleanup_status=0 container
    trap - EXIT
    if [[ "$activation_started" == true ]]; then
        if ! host_state restore > "$EVIDENCE/activation-restore.log" 2>&1; then cleanup_status=1; fi
    fi
    # A failed restore may mean an owned VM/daemon is still alive. Do not remove
    # its backing pool or send a broad kill; retain everything for diagnosis.
    if [[ "$pool_created" == true && "$cleanup_status" == 0 ]]; then
        if [[ -f "$EVIDENCE/pool/ownership.json" ]] && ! python3 scripts/legacy_snapshot_pool.py remove --receipt "$EVIDENCE/pool/ownership.json" > "$EVIDENCE/pool-cleanup.log" 2>&1; then cleanup_status=1; fi
        if sudo -n dmsetup info "$FICD_DM_POOL" > /dev/null 2>&1; then cleanup_status=1; fi
    fi
    if [[ -f "$EVIDENCE/existing-containers.txt" ]]; then
        while IFS= read -r container; do
            [[ -z "$container" ]] && continue
            if [[ "$(docker inspect --format '{{.State.Running}}' "$container")" != true ]]; then cleanup_status=1; fi
        done < "$EVIDENCE/existing-containers.txt"
    fi
    printf 'cleanup\t%s\n' "$cleanup_status" >> "$STATUS"
    test "$cleanup_status" = 0 || result=1
    if [[ "$result" == 0 ]]; then echo VHIVE_FINAL_INSTALLED_CONSUMER_OK > "$EVIDENCE/completion-marker"; fi
    printf 'launcher\t%s\n' "$result" >> "$STATUS"
    exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
run_logged host-before host_state before
run_logged vhive-source git rev-parse HEAD HEAD^{tree}
run_logged existing-containers docker ps --no-trunc -q
cp "$EVIDENCE/existing-containers.log" "$EVIDENCE/existing-containers.txt"
sudo -n test -r /dev/kvm
sudo -n test -w /dev/kvm
if sudo -n dmsetup info "$FICD_DM_POOL" > /dev/null 2>&1; then echo 'Pool already exists; refusing reuse' >&2; exit 1; fi

# Resolve the actual committed published replacements BEFORE installation or
# global activation; development overrides and temporary modfiles are forbidden.
run_logged published-modules python3 - "$GO_BIN" "$RUNTIME_REVISION" "$SDK_REVISION" "$EVIDENCE" "$RUNTIME_REPOSITORY" "$RUNTIME_REF" <<'PY'
import importlib.util,json,os,pathlib,subprocess,sys,tempfile
go,runtime,sdk,out,repository,reference=sys.argv[1:];sys.dont_write_bytecode=True

def require_publication(repository, reference, revision):
    # Query the canonical public remote outside any checkout. Do not let an
    # inherited Git config rewrite the URL to an unpublished local repository.
    environment={k:v for k,v in os.environ.items() if not k.startswith('GIT_')}
    environment.update(GIT_CONFIG_NOSYSTEM='1',GIT_CONFIG_GLOBAL=os.devnull,GIT_TERMINAL_PROMPT='0')
    with tempfile.TemporaryDirectory(prefix='vhive-published-ref-') as directory:
        advertised=subprocess.check_output(['git','ls-remote','--exit-code',repository,reference],cwd=directory,env=environment,text=True,timeout=60)
    assert advertised.splitlines()==[revision+'\t'+reference],'Reviewed Step2 revision is not the exact published integration branch head'
    return {'repository':repository,'ref':reference,'revision':revision,'advertised':advertised}

def require_module_origin(record, path, revision):
    replacement=record['selected'].get('Replace',{})
    downloaded=record['downloaded'];origin=downloaded.get('Origin',{})
    assert replacement.get('Path')==path and replacement.get('Version'),'Expected a versioned canonical module replacement'
    assert downloaded.get('Path')==path and downloaded.get('Version')==replacement['Version'],'Downloaded module differs from selected replacement'
    assert not downloaded.get('Error'),'Module download failed'
    assert origin.get('VCS')=='git' and origin.get('URL','').removesuffix('.git')=='https://'+path,'Module origin is not the canonical published repository'
    assert origin.get('Hash')==revision,'Module origin differs from the reviewed immutable revision'

publication=require_publication(repository,reference,runtime)
spec=importlib.util.spec_from_file_location('accept',pathlib.Path('scripts/accept_legacy_snapshot.py'));a=importlib.util.module_from_spec(spec);spec.loader.exec_module(a)
manifest=json.loads(pathlib.Path('configs/setup/firecracker-stack.json').read_text())
assert manifest['runtime']['ready'] is True and manifest['runtime']['revision']==runtime,'Final ready manifest must select the explicitly reviewed published Step2'
assert manifest['runtime']['repository']==repository,'Manifest runtime repository differs from the canonical publication remote'
assert manifest['inputs']['sdk']['commit']==sdk
records={}
for original,path,revision in [('github.com/firecracker-microvm/firecracker-containerd','github.com/vhive-serverless/firecracker-containerd',runtime),('github.com/firecracker-microvm/firecracker-go-sdk','github.com/vhive-serverless/firecracker-go-sdk',sdk)]:
    record=a.module_origin(go,original,revision,dict(os.environ))
    require_module_origin(record,path,revision)
    records[original]=record
pathlib.Path(out,'published-modules.json').write_text(json.dumps(records,indent=2)+'\n')
pathlib.Path(out,'published-runtime-ref.json').write_text(json.dumps(publication,indent=2)+'\n')
# Go test executables may omit dependency entries from `go version -m`.
# Record the committed test graph's actual runtime/SDK package directories too.
template='{{if .Module}}{{.ImportPath}}\t{{.Module.Path}}\t{{with .Module.Replace}}{{.Path}}\t{{.Version}}\t{{.Dir}}{{end}}\t{{.Dir}}{{end}}'
listing=subprocess.check_output([go,'list','-mod=readonly','-deps','-test','-f',template,'./ctriface'],text=True)
packages=[]
for line in listing.splitlines():
    fields=line.split('\t')
    if len(fields)<2 or fields[1] not in records: continue
    assert len(fields)==6,fields
    record=records[fields[1]];replacement=record['selected']['Replace']
    assert fields[2]==replacement['Path'] and fields[3]==replacement['Version']
    assert pathlib.Path(fields[4]).resolve()==pathlib.Path(record['downloaded']['Dir']).resolve()
    assert pathlib.Path(fields[5]).resolve().is_relative_to(pathlib.Path(fields[4]).resolve())
    packages.append(dict(zip(['package','module','replacement','version','replacement_directory','package_directory'],fields)))
# The consumer imports runtime APIs; the runtime binary uses the selected SDK.
# SDK origin is verified above even when no SDK package is in this test graph.
counts={module:sum(p['module']==module for p in packages) for module in records}
assert counts['github.com/firecracker-microvm/firecracker-containerd']>0
pathlib.Path(out,'test-dependency-package-counts.json').write_text(json.dumps(counts,indent=2)+'\n')
pathlib.Path(out,'test-dependency-packages.json').write_text(json.dumps(packages,indent=2)+'\n')
subprocess.run([go,'mod','verify'],check=True)
PY
run_logged manifest python3 scripts/firecracker_stack.py validate
run_logged installer-unit python3 -m unittest discover -s scripts/tests -v
# The worker is already provisioned. Avoid --prepare: it apt-installs docker.io
# and other host packages, which is inappropriate on a shared Kubernetes host.
# Missing route is a fail-closed prerequisite, never a nil-manager test crash.
run_logged route command -v route
mkdir -p "$STACK_DIR/toolchains"
ln -s "$(dirname "$(dirname "$GO_BIN")")" "$STACK_DIR/toolchains/go1.26.11"
run_logged install python3 scripts/firecracker_stack.py install
run_logged verify python3 scripts/firecracker_stack.py verify
cp "$INSTALL_DIR/vhive-stack-receipt.json" "$EVIDENCE/"
runtime_source="$STACK_DIR/sources/$RUNTIME_REVISION"
test "$(git -C "$runtime_source" rev-parse HEAD)" = "$RUNTIME_REVISION"
test -z "$(git -C "$runtime_source" status --porcelain)"
run_logged installed-runtime-origin python3 - "$runtime_source" "$RUNTIME_REVISION" "$RUNTIME_REPOSITORY" "$EVIDENCE" <<'PY'
import hashlib,json,pathlib,subprocess,sys
source,revision,repository,out=sys.argv[1:]
def git(*args): return subprocess.check_output(['git','-C',source,*args],text=True).strip()
origin=git('remote','get-url','origin')
assert origin.removesuffix('.git')==repository.removesuffix('.git'),'Installed runtime was fetched from another repository'
assert git('rev-parse','HEAD')==revision and not git('status','--porcelain'),'Installed runtime source identity or cleanliness changed'
modules=json.loads(pathlib.Path(out,'published-modules.json').read_text())
module=modules['github.com/firecracker-microvm/firecracker-containerd']['downloaded']
lock='docs/modernization/versions.json'
source_hash=hashlib.sha256(pathlib.Path(source,lock).read_bytes()).hexdigest()
assert source_hash==hashlib.sha256(pathlib.Path(module['Dir'],lock).read_bytes()).hexdigest(),'Installed source and selected module input locks differ'
receipt={'repository':origin,'revision':revision,'tree':git('rev-parse','HEAD^{tree}'),'runtime_lock_sha256':source_hash,'module_version':module['Version']}
pathlib.Path(out,'installed-runtime-origin.json').write_text(json.dumps(receipt,indent=2)+'\n')
print(json.dumps(receipt,indent=2))
PY
pool_created=true
run_logged pool-create python3 scripts/legacy_snapshot_pool.py create --runtime-source "$runtime_source" --pool "$FICD_DM_POOL" --evidence "$EVIDENCE/pool"
activation_started=true
run_logged activate python3 scripts/firecracker_stack.py activate --pool "$FICD_DM_POOL"
run_logged installed-acceptance sudo -n env "PATH=$PATH" GOWORK=off GOTOOLCHAIN=local GOENV=off GOFLAGS= \
    python3 scripts/accept_legacy_snapshot.py --prefix "$INSTALL_DIR" \
    --runtime-revision "$RUNTIME_REVISION" --go "$GO_BIN" --output "$EVIDENCE/acceptance"
grep -q '^VHIVE_LEGACY_SNAPSHOT_ACCEPTANCE_OK receipt=' "$EVIDENCE/installed-acceptance.log"
sha256sum "$EVIDENCE/acceptance/ctriface.test" > "$EVIDENCE/test-binary.sha256"
python3 - "$EVIDENCE/acceptance/acceptance.json" "$EXPECTED_VHIVE_REVISION" "$RUNTIME_REVISION" <<'PY'
import json,pathlib,sys
receipt=json.loads(pathlib.Path(sys.argv[1]).read_text())
assert receipt['result']=='pass' and receipt['candidate'] is False
assert receipt['vhive_revision']==sys.argv[2] and receipt['runtime_revision']==sys.argv[3]
PY
test "$(git rev-parse HEAD)" = "$EXPECTED_VHIVE_REVISION"
test -z "$(git status --porcelain)"
# The EXIT trap restores owned activation, removes the owned pool, checks prior
# running containers and only then emits the separate launcher completion marker.
