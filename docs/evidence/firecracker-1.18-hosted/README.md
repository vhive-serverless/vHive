# Hosted Firecracker 1.18 integration acceptance

All four hosted workflows completed successfully on published vHive source
`2120fc8a9855b25a19cde15013f2bb581924bc3f` (tree
`f6fd7163169567d24888692b1f16592e257f8663`) on 2026-11-13. This later
receipt/documentation tip was not the source used by those jobs; their results
belong to the named source, without a recursive CI claim for documentation tips.

| Workflow | Run / result |
| --- | --- |
| Build and code quality | [43354889200: success](https://github.com/vhive-serverless/vHive/actions/runs/43354889200) |
| Unit tests | [43354889247: success](https://github.com/vhive-serverless/vHive/actions/runs/43354889247) |
| Setup-script builds | [43354889075: success](https://github.com/vhive-serverless/vHive/actions/runs/43354889075) |
| Standalone legacy snapshots, File memory | [43354889183: success](https://github.com/vhive-serverless/vHive/actions/runs/43354889183) |

Original [run metadata and complete logs](runs/) retain exact source/job/step
results. The compact [four-run inventory](fixes/vhive-ci-fixes-runs.json) includes
the later-completing snapshot job; the earlier `completed-checks.json` records
only the first three completed collections.

The four active unit jobs (`misc`, `networking`, `snapshotting`, and
`memory/manager`) passed. The profile job and old Firecracker-containerd
interface matrix remained skipped; the separate File-memory snapshot workflow
provides the migrated profile's integration coverage.

## Fresh installed stack and VM results

The snapshot job built and installed published runtime
`b4f8c6c2f1ec820ee4e8c2aa8daa3ade98d5f794` afresh on its disposable Ubuntu
24.04 x86_64 KVM runner, including the guest kernel and rootfs. This differs
from the [earlier `da498d75` installed-worker receipt](../firecracker-1.18-installed/README.md),
which explicitly reused and reverified its preceding installation. Each receipt
retains its own consumer, build artifacts and cleanup scope.

| Required unchanged VM case | Result |
| --- | --- |
| `TestPauseSnapResume` | PASS, 22.73 s |
| `TestSnapshotConcurrentClones` | PASS, 143.02 s |
| `TestSnapshotSourceMemoryAndDiskIsolation` | PASS, 78.12 s |

The [final acceptance receipt](snapshot/acceptance/acceptance.json) records
`candidate=false`, published canonical runtime and SDK module origins, source
and rendered-configuration hashes, and the exact compiled test command.
The [test log](snapshot/acceptance/tests.log) records all three actual PASS
lines: continued source operation, independent live clones and shutdown, and
guest memory/process continuity with source/clone disk isolation. The profile
remained devmapper, File memory, UPF disabled, and the pinned
`ghcr.io/ease-lab/helloworld@sha256:12dc6715ed1a8306f246ceaf7742c09e38a52a79d17421e4a50d7e0e09fdbc25`
image. Runtime `b4f8` and SDK `dd9db322432989b96cff7a38a64703d60b6cabf9`
module pins are unchanged.

The [installation receipt](snapshot/vhive-stack-receipt.json) and
[fresh build/install log](snapshot/install.log) record these installed SHA-256s:

| Artifact | SHA-256 |
| --- | --- |
| Official unmodified Firecracker v1.18.0 | `b33f46cd6d88834cf6a4f8d184f08f219809c636236bffd286d233a60602a2ba` |
| Official jailer v1.18.0 | `ddadbaa15b5c942271a83b7040ca7259c8ccf92a9e08d44cce0b86fa407ba460` |
| Newly built kernel | `ba27d05bd463258b38c19646275ac68de667dc10d6533c200af2163fd1d63665` |
| Newly built rootfs | `6079730b3bdcfe39fcd7a4dd4fc718fecff91a8a6a6c68d950ae6a769e4a2faf` |

The VMM/jailer hashes match the earlier official release binaries; the new
kernel/rootfs hashes identify this build rather than implying equality with
the worker's older images. containerd/runc remain v2.3.7/v1.4.2. The excluded
compiled test executable was 32,385,472 bytes, SHA-256
`6317986c84c8659cf419531f6111ace03fe98385365d5c41984c54ddcb21508c`;
its [build metadata](snapshot/acceptance/test-binary-buildinfo.txt) is retained.

The [private-pool receipt](snapshot/pool/ownership.json) records 40GiB data,
1GiB metadata, unchanged 10GiB disks, three simultaneous VMs and image budget
four, with `removed=true`. The [cleanup log](snapshot/pool-cleanup.log),
[test log](snapshot/acceptance/tests.log) and successful final job document owned
pool removal and test/daemon shutdown on the disposable runner. This is not an
independent inspection of the eight historical daemons/Kubernetes container on
the earlier worker; that separate inspection remains in its original receipt.

## CI failures investigated and corrected

[Three original failed-job logs](failures/) remain unchanged. Build/unit
runs `43352640912`/`43352640942` exposed the workspace's old monolithic
`google.golang.org/genproto` conflicting with the pinned RPC submodule. Commit
`2120fc8a` maps that root to canonical upstream
`v0.0.0-20230526161137-0005af68ea54`, which has no overlapping RPC directory.
The existing RPC provider and runtime/SDK pins remain unchanged. The
[663-entry standalone closure comparison](fixes/standalone-closure-comparison.json)
found no changed package identities, dependencies or source inputs: the earlier
standalone worker acceptance is not relabeled as workspace coverage. The
[original fix report](fixes/vhive-genproto-fix-report.md) and
[local results](fixes/final-result.json) retain their preparation-time status;
the successful hosted runs above supply the later result.

Snapshot run `43352640967` failed before build when the generic runtime
prerequisite script tried to install Ubuntu Docker packages over hosted Docker
CE's `containerd.io`. Commit `88a779314f8e74e9e94d1cd59627f379ab0d0691`
adds CI-specific prerequisite provisioning: retain working Docker/buildx, install
the runtime's other build dependencies plus `net-tools` with `--no-remove`,
verify the manifest-pinned Go 1.26.11 archive/toolchain path, and invoke the
receipt-backed installer without `--prepare`. The
[fix report](fixes/vhive-hosted-snapshot-prerequisites-report.json),
[five isolated prerequisite tests](fixes/vhive-hosted-snapshot-prerequisites-tests.log)
and [actual hosted prerequisite log](snapshot/prerequisites.log) record this
correction. No runtime changes or reduced VM/assertion/timeout criteria were
used to resolve either failure.

## Reproduction, preservation and limits

Use a clean checkout of exact `2120fc8a` on a disposable Ubuntu 24.04 x86_64
KVM runner with usable Docker/buildx and passwordless sudo. Supply new private
`STACK_DIR`, `INSTALL_DIR`, `VHIVE_CI_EVIDENCE` paths and a unique `FICD_DM_POOL`.
The [committed workflow](../../../.github/workflows/legacy_snapshot_tests.yml)
checks KVM, reclaims runner build space, and orders these commands:

```sh
bash scripts/github_runner/prepare_legacy_snapshot.sh
bash scripts/github_runner/test_legacy_snapshot.sh
```

The [earlier integration handoff](../firecracker-1.18-installed/README.md#published-migration-handoff)
records all four runtime branches, SDK publication, history reconstruction and
recovery tags. Their preparation-time consumer-pending notices are closed by
the named installed and hosted receipts, without changing those immutable pins.
The original Step 2 full run remains exit 2 on the **unfixed CNI chain race**,
with complete segmented coverage recorded separately; this consumer pass does
not convert it into an uninterrupted pass. Full CRI remains WIP, Kubernetes
whole-Pod checkpoint/restore endpoints remain deferred to 1.38, and these runs
supply no new performance or memory-saving measurements.

[Collection manifest](collection-manifest.json) records every imported source,
byte count and hash, plus the excluded ELF executable. [SHA256SUMS](SHA256SUMS)
covers all retained text receipts/logs and this authored summary. No binary,
kernel or rootfs image is duplicated in this archive. Original pending/uncommitted
statements inside frozen reports describe their own preparation time.
