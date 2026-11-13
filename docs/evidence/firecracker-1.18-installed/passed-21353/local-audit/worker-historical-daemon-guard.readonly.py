import os,pathlib,json,hashlib
allowlist_path=__import__("sys").argv[1]
bins={'firecracker', 'demux-snapshotter', 'firecracker-containerd', 'jailer', 'containerd-shim-aws-firecracker', 'http-address-resolver', 'firecracker-ctr'}
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

idle()
print("EXACT_HISTORICAL_DAEMON_GUARD_OK")
