# Installed Firecracker 1.18 consumer acceptance

## Exact accepted source and result

On 2026-11-13, installed-consumer run `21353` completed with exit 0 and
`VHIVE_FINAL_INSTALLED_CONSUMER_OK`. The clean tested vHive source was
`da498d75b7b91f9dcfcf3e2f80107b48f0cbdee2` (tree
`7cb3d4513a57981ea2cff627a2b158f1deb42aff`), with the published Step 2 runtime
`b4f8c6c2f1ec820ee4e8c2aa8daa3ade98d5f794`. This receipt is a later
documentation-only addition; that documentation tip was not the source used to
compile or run the VM tests.

| Required standalone File-memory case | Result |
| --- | --- |
| `TestPauseSnapResume` | PASS, 24.71 s |
| `TestSnapshotConcurrentClones` | PASS, 117.21 s |
| `TestSnapshotSourceMemoryAndDiskIsolation` | PASS, 78.32 s |

The tests cover continued source task/control/output after capture, two
independent live clones with separate endpoints/disks and shutdown, and guest
memory/process continuity and source/clone disk isolation. The final acceptance
script used its default final mode (`candidate=false`), devmapper, File memory,
UPF disabled, and the immutable function image
`ghcr.io/ease-lab/helloworld@sha256:12dc6715ed1a8306f246ceaf7742c09e38a52a79d17421e4a50d7e0e09fdbc25`.
See [the original acceptance receipt](passed-21353/evidence/acceptance/acceptance.json),
[VM log](passed-21353/evidence/acceptance/tests.log),
[phase statuses](passed-21353/evidence/commands.tsv), and
[independent terminal/cleanup report](vhive-installed-final-21353-terminal-report.json).

The runtime preparation documents at the four published tips below recorded
final consumer acceptance as pending. This receipt closes that separate gate
for the named consumer and published runtime. It does not expand the scope of
the earlier runtime or provisional local-replacement receipts, or imply those
preparation documents already contained a final consumer result. The accepted
runtime/module pins and cumulative branch histories remain unchanged.

## Installed binaries and published module origins

The run consumed canonical published modules, with no local runtime replacement,
temporary modfile, development override or candidate acceptance mode:

| Component | Selected version / exact origin |
| --- | --- |
| Runtime | `github.com/vhive-serverless/firecracker-containerd v0.0.0-20261113011217-b4f8c6c2f1ec`, origin `b4f8c6c2f1ec820ee4e8c2aa8daa3ade98d5f794` |
| SDK | `github.com/vhive-serverless/firecracker-go-sdk v0.0.0-20261112180021-dd9db3224329`, origin `dd9db322432989b96cff7a38a64703d60b6cabf9` |
| VMM / jailer | Official, unmodified Firecracker v1.18.0 release binaries |
| containerd / runc | v2.3.7 / v1.4.2 |
| Test toolchain | Go 1.26.11, Linux amd64 |

The runtime installation was built successfully during failed run `18560`.
Run `21353` **reused and reverified** that exact installed prefix, its ready
manifest/input lock and artifact hashes; it did not perform a second runtime,
kernel or rootfs build. The reused installation receipt has SHA-256
`9cd10430db5acea75593df0193d5987698ee5990b7765a18bc7e0556ba05d1e4`.
See [reused receipt](passed-21353/evidence/reused-install-receipt.json),
[verification](passed-21353/evidence/verify.log),
[installed source origin](passed-21353/evidence/installed-runtime-origin.json),
and [canonical module origins](passed-21353/evidence/published-modules.json).

| Installed artifact | SHA-256 |
| --- | --- |
| Firecracker | `b33f46cd6d88834cf6a4f8d184f08f219809c636236bffd286d233a60602a2ba` |
| jailer | `ddadbaa15b5c942271a83b7040ca7259c8ccf92a9e08d44cce0b86fa407ba460` |
| kernel | `a7000bfbbaabee244e1e6e3e8ba09ba0c69037a63601b0121845a2f26ef097c7` |
| installed rootfs | `cb40334f5f6d5a91487eb6067805d1a78ea2bc4275d1276ef3a482c548a0c804` |

The compiled consumer test binary has its own
[hash](passed-21353/evidence/test-binary.sha256) and
[build metadata](passed-21353/evidence/acceptance/test-binary-buildinfo.txt).
The [actual test dependency package inventory](passed-21353/evidence/test-dependency-packages.json)
supplements Go test build metadata, which can omit dependency entries. SDK
selection/origin verification is separate from the set of SDK packages imported
by the consumer test binary. No claim equates this installed rootfs to rootfs
artifacts from the earlier runtime acceptance pipelines.

## Preserved failure and capacity correction

Run `18560` tested consumer `7efebc4bd0061065f73ffb1527bfdf4c084f2628`
against the same published runtime. `TestPauseSnapResume` passed (25.34 s), but
concurrent clones (133.47 s) and memory/disk isolation (131.44 s) failed with
`dd`/`fsync` EIO. The private test pool exhausted all 163840 data blocks
(`out_of_data_space`): the generic runtime helper's 10GiB pool could not hold
the unchanged 10GB disks (parsed as 10GiB) and independent full clone copies.
The failure, original receipts, logs and capacity diagnosis remain immutable in
[failed-18560](failed-18560/), with its
[terminal report](vhive-installed-final-18560-terminal-report.json).

Consumer `da498d75` adds the shared test-pool provisioner used by CI and the
reviewed worker launcher: fresh private 40GiB data / 1GiB metadata, at least
43GiB available host storage, owned-resource receipts, existing-resource
rejection and capacity verification before testing. The
[actual capacity/ownership receipt](passed-21353/evidence/pool/ownership.json)
records the provisioned sizes and subsequent removal. The existing product
setup's 100GiB pool was already adequate. No runtime source, disk size, test
assertion, timeout or VM count was reduced to obtain the passing result.

Cleanup returned 0 after both attempts. The final independent check confirmed
owned VM/daemon/device-map/loop/mount/open-file resources were gone and the ten
activation paths were restored. Eight previously audited idle historical
runtime daemons and the unrelated Kubernetes control-plane container were
preserved. The host-specific exact-identity allowlist was checked, rather than
stopping those processes or weakening the ownership guard. Retained historical
state and previously approved quarantine are documented in the failed-attempt
audit; they were not deleted to obtain acceptance.

## Reproduction and archived commands

Use the exact tested consumer above, Python 3.11+, Go 1.26.11, Linux x86_64
KVM, passwordless sudo, Docker, and `route` from `net-tools`. Reserve exclusive
use of this profile's endpoints and global activation paths. Supply new private
`STACK_DIR`, `INSTALL_DIR`, `VHIVE_CI_EVIDENCE` directories and a unique
`FICD_DM_POOL`; a clean source checkout and published pins remain required.

For the current hosted CI sequence (updated after the exact worker run above),
first run [`scripts/github_runner/prepare_legacy_snapshot.sh`](../../../scripts/github_runner/prepare_legacy_snapshot.sh)
on the disposable KVM host. It requires usable Docker and buildx, preserves the
existing Docker installation, installs the remaining build dependencies, and
verifies the manifest-pinned Go archive. Then run
[`scripts/github_runner/test_legacy_snapshot.sh`](../../../scripts/github_runner/test_legacy_snapshot.sh)
for receipt-backed installation/activation, shared pool creation, final
acceptance, and owned pool cleanup. The
[hosted workflow](../../../.github/workflows/legacy_snapshot_tests.yml) records
this ordering. The historical worker launcher below remains the exact command
record for the accepted `da498d75` source.

The executed [worker launcher](passed-21353/local-audit/vhive-installed-consumer-capacity-reuse.prepared.sh)
is preserved byte-for-byte despite its historical `prepared` filename/comment.
It records the exact ordered module/source checks, installation reuse,
activation backup/restore, three-case execution and cleanup. Its recorded worker paths and required selections were:

```sh
export EXPECTED_VHIVE_REVISION=da498d75b7b91f9dcfcf3e2f80107b48f0cbdee2
export EXPECTED_PUBLISHED_STEP2_REVISION=b4f8c6c2f1ec820ee4e8c2aa8daa3ade98d5f794
export VHIVE_CHECKOUT=/users/nehalem/fc-ctrd-mod/vhive-final-consumer-da498d75
export GO_BIN=/absolute/path/to/go1.26.11/bin/go
export RUN_DIR=/users/nehalem/fc-ctrd-mod/vhive-installed-final-da498d75-20261113
export VHIVE_HISTORICAL_DAEMON_ALLOWLIST=/path/to/worker-historical-daemons.exact-allowlist.json
bash /path/to/vhive-installed-consumer-capacity-reuse.prepared.sh
```

This is an execution record, not a command to rerun against the occupied
historical paths: the launcher rejects an existing run directory. Its reused
prefix and [allowlist](passed-21353/local-audit/worker-historical-daemons.exact-allowlist.json) are tied
to that worker and boot, and intentionally fail on another host. For a new
worker use a fresh installation and owned paths via the committed CI sequence;
do not copy or relax a historical process allowlist.

## Later hosted acceptance

The [hosted acceptance receipt](../firecracker-1.18-hosted/README.md) records all
four CI workflows passing at exact `2120fc8a`, including a fresh runtime/kernel/
rootfs build and the same three VM cases. This earlier receipt retains its
exact `da498d75` consumer, reused installation and independently inspected
historical-worker cleanup scope.

## Published migration handoff

| Repository / branch | Published commit | Build CI |
| --- | --- | --- |
| SDK `firecracker-v1.18.0` | `dd9db322432989b96cff7a38a64703d60b6cabf9` | SDK validation retained separately |
| Runtime `firecracker-v1.18-containerdv2` | `71925ad2d0dcb0b717fa0b07479543bd5ecb267a` | [success](https://github.com/vhive-serverless/firecracker-containerd/actions/runs/43345610239) |
| Runtime `firecracker-v1.18-containerdv2-vhive-integration` | `b4f8c6c2f1ec820ee4e8c2aa8daa3ade98d5f794` | [success](https://github.com/vhive-serverless/firecracker-containerd/actions/runs/43345609946) |
| Runtime `cri-native-snapshot-restore` | `64452aaa028d8b4d291f5ecf9077f3ecd649237e` | [success](https://github.com/vhive-serverless/firecracker-containerd/actions/runs/43345610265) |
| Runtime `damon-virtio-pmem` | `7332985cee734763ed1d820685e85390db3c8f6c` | [success](https://github.com/vhive-serverless/firecracker-containerd/actions/runs/43345610469) |

The [47-commit assembly map](final-release-assembly-map.json) records the
reconstructed incremental history. The
[56-tag source/original-history inventory](source-archive-publication.tsv) and
[archive publication receipt](source-archive-publication-receipt.json) preserve
recoverable commits/trees. The
[runtime publication receipt](runtime-final-publication-receipt.json),
[fresh-fetch verification](final-release-fresh-remote-verification.json), and
[five-obsolete-branch cleanup receipt](obsolete-five-deletion-receipt.json)
record guarded publication and recovery; other historical/ambiguous branches
were retained. The vHive integration branch is
`firecracker-v1.18-containerdv2-integration`, distinct from the runtime branch
with the `-vhive-integration` suffix. This receipt does not itself assert vHive
publication or a hosted vHive workflow result; those need their own exact-SHA
publication/CI records.

## Limits and preservation

Step 2 runtime acceptance remains complete segmented coverage with its original
full-run exit 2 on an **unfixed CNI chain-creation race**; unchanged five-VM
repeats and the remaining thinpool/SDK tail passed separately. This installed
consumer pass does not turn that original full run into an uninterrupted pass.
Step 3 native/real-volume/focused-CRI acceptance and Step 4 full-stack/unique
component acceptance retain their published exact-source scopes. Full CRI is
WIP; Kubernetes whole-Pod checkpoint/restore endpoints remain deferred to 1.38.
UPF/lazy memory, native/CRI restore through this consumer, other architectures
and new performance or memory-saving measurements are not established here.

All copied logs/JSON/receipts retain original bytes, including failed and
preparation-time pending statements. [SHA256SUMS](SHA256SUMS) covers this bounded
text archive and authored receipt. Collection manifests record the hashes of
large binaries, guest images and the test executable without copying them.
Existing published runtime raw logs are linked through their runtime receipts
rather than duplicated here.
