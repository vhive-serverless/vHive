# Firecracker snapshots

## Overview

We
support [vanilla Firecracker snapshots](https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/snapshot-support.md)
and a snapshotting technique based on the snapshot API Firecracker offers.

There are 2 modes of snapshot operation: local and remote. While the local mode is stable and fully operational
(with a small issue, namely, GH-818), the remote mode is currently
too [unstable](./snapshots.md#container-disk-state-restoration-blocker)
to be used.

There are two modes of snapshot operation: **local** and **remote**.

- **Local mode** is stable and fully operational, except a minor known issue (GH-818). It works both with `stargz` and
  `devmapper` containerd snapshotters.

- **Remote mode** is now **partially operational**:
    - It works reliably when using [**stargz**](https://github.com/containerd/stargz-snapshotter) as
      the [remote snapshotter](https://github.com/firecracker-microvm/firecracker-containerd/blob/main/docs/remote-snapshotter.md).
    - However, remote snapshots with `devmapper`
      remain [unstable](./snapshots.md#container-disk-state-restoration-blocker) and is **not recommended** for
      production use.

---

## Local snapshots

The current approach allows loading an arbitrary amount of VMs from a single snapshot. This is done by creating a new
shim and the required block and network devices upon loading a snapshot and creating an extra patch file containing the
filesystem differences written by the VM upon snapshot creation. In addition, the snapshotting operation can be
further configured using the following flags:

- `netPoolSize [capacity]`: the amount of network devices in the Firecracker VM network pool (`10` by default), which
  can be used to keep the network initialization off the cold start path of Firecracker VMs.

### Snapshot creation

Snapshots are created using the following steps:

1. Pause the VM.
2. Create a Firecracker VM snapshot (memory and CPU state).
3. **Capture container snapshot (disk state) changes**, only if using **`devmapper`**:
    1. Get a snapshot of the original container image.
    2. Mount the original container image snapshot.
    3. Mount the current container snapshot.
    4. Use `rsync --only-write-batch` to generate a patch file with the differences between the two snapshots.
4. Resume the VM.

> ℹ️ If using `stargz`, capturing disk changes is not needed. The container's root filesystem is mounted
> via a **FUSE filesystem**, and its state is persisted in the memory snapshot.  
> See the
> [remote snapshotter architecture](https://github.com/firecracker-microvm/firecracker-containerd/blob/main/docs/remote-snapshotter.md)
> for details about this architecture.

### Snapshot loading

To load a local snapshot:

1. **Restore container snapshot (disk state)** (only required if using `devmapper`):
    1. Retrieve a snapshot of the original container image.
    2. Mount the original container image snapshot.
    3. Apply changes from the patch file to the mounted container snapshot using `rsync --read-batch`.

2. **Create a VM** from the snapshot by providing:
    - The memory snapshot file
    - The VM state file
    - The path to the patched container snapshot (via `ContainerSnapshotPath`)

> ℹ️ If using `stargz`, container snapshot restoration is **not** required, as the container rootfs is mounted
> via FUSE and restored directly from the memory snapshot.

### Setup

To enable local snapshots, you just need to start the vHive server with the `-snapshots 'local'` flag.

---

## Remote snapshots

Rather than only using the snapshots available locally on a node, snapshots can also be transferred between nodes to
potentially accelerate cold start times and reduce memory utilization, given that proper mechanisms are in place to
minimize the snapshot network transfer latency. This could be done by storing snapshots in a global storage solution
such as [MinIO S3](./developers_guide.md#MinIO-S3-service), or directly distributing snapshots between compute nodes.

> ℹ️ Remote snapshots currently are only supported when using the `stargz` snapshotter.

### Snapshot creation

Snapshots are created using the same algorithm as for local snapshots with an additional upload step (uploading
the snapshot files to the global storage solution).

### Snapshot loading

Snapshots are loaded using the same algorithm as for local snapshots with a preliminary download step (downloading
the snapshot files from the global storage solution).

### Setup

To enable remote snapshots, first you need to set up a global storage solution (e.g., MinIO S3) to store the snapshots. 
Check how to set up MinIO S3 in [here](./developers_guide.md#MinIO-S3-service).

Since remote snapshots currently only work with the `stargz` snapshotter, you need to set up the node with this in mind:

```bash
./setup_tool setup_node firecracker use-stargz
```

This will set up the node with the `stargz` snapshotter and the required configuration to use remote snapshots.

Start both `demux-snapshotter` and the `http-address-revolver` in the background:

```bash
sudo demux-snapshotter 
```

and

```bash
sudo http-address-revolver
```

Finally, when starting the vHive server, you need to specify some extra flags:

```bash
./vhive -snapshots 'remote' \       # use remote snapshots
    -snapshotter 'proxy' \          # use the proxy snapshotter (demux-snapshotter)
    -dockerCredentials '{"docker-credentials":{"ghcr.io":{"username":"...","password":"..."}, "https://index.docker.io/v1/":{"username":"...","password":"..."}}}' # credentials for pulling images
```

> ℹ️ Check the firecracker-containerd [remote snapshotter documentation](https://github.com/firecracker-microvm/firecracker-containerd/blob/main/docs/remote-snapshotter.md) for more details about the remote snapshotter architecture.

### Blockers

#### Container disk state restoration blocker

##### Current state

Currently, an experimental [proof of concept
branch](https://github.com/vhive-serverless/vHive/tree/remote-firecracker-snapshots-poc) exists, where one can find the
PoC in
the [remote-firecracker-snapshots-poc](https://github.com/vhive-serverless/vHive/tree/remote-firecracker-snapshots-poc/remote-firecracker-snapshots-poc)
folder. Instructions on setting up and working with the PoC are provided in the folder's
[README](https://github.com/vhive-serverless/vHive/blob/remote-firecracker-snapshots-poc/remote-firecracker-snapshots-poc/README.md).

##### Outline

Currently, the blocker for using remote snapshots with `devmapper` is container disk state restoration. Containers restored on a
clean node seem to be healthy, and respond to requests, but their disk state gets corrupted after a request is received.

Corruption symptoms vary among different containers:

* a
  nginx [container](https://hub.docker.com/layers/library/nginx/1.17-alpine/images/sha256-781d4ec0559e7c679d54078b60efd925f260ef45275dd0155c986fa2c0511791?context=explore)
  returns internal server error response and the Firecracker kernel log contains ext4 checksum errors;
* a simple Python 'Hello world' HTTP
  server [container](https://hub.docker.com/layers/kaustavdas1987/hello-world-python/0.0.3.RELEASE/images/sha256-ad8b918f7aa79cc3b59d6d8dfa99623064853f8c8227a5ec4677958ee63b8c5e?context=explore)
  crashes and the Firecracker log contains a ‘trap invalid opcode’ error from the Python interpreter;
* a simple Golang 'Hello world' HTTP
  server [container](https://hub.docker.com/layers/qorbani/golang-hello-world/latest/images/sha256-a14f3fbf3d5d1c4a000ab2c0c6d5e4633bdb96286a0130fa5b2c5967b934c31f?context=explore)
  works, but if we rebuild the same binary, it crashes (judging by the exit code, looks like the Golang runtime panics).

The potential root cause of this problem in Firecracker was discussed in
[firecracker-microvm/firecracker#4036](https://github.com/firecracker-microvm/firecracker/issues/4036). Incrementally
debugging various manual firecracker setups, including:

* running a binary from a rootfs;
* running a binary from an attachable drive backed by a regular file;
* running a binary from an attachable drive backed by a thin device mapper;
* running a binary from an attachable drive backed by a containerd snapshot;

with everything working fine drove us to the conclusion that the problem is not caused by the VM snapshot loading
process and is rather inherent to firecracker-containerd. This issue is currently tracked in
[firecracker-microvm/firecracker-containerd#759](https://github.com/firecracker-microvm/firecracker-containerd/issues/759).

Having studied all firecracker-containerd interactions with container snapshots and Firecracker (both the VM and the
agent running in the VM), we did not find any problems and any special filesystem actions other than those that we did
manually, which led us to the conclusion that the problem may be related to the shim and container filesystem management
inside the VM.

##### Observations

* The container disk state gets corrupted exactly after the moment a request is sent to it, i.e., it is healthy between
  the moments the VM is loaded from a snapshot and the request to the container is sent.
* A change in the number of major page faults for the nginx process between loading the VM from a snapshot and sending a
  request to the container was not observed (this was not checked for a Golang server, as it crashes, and a way to track
  the page fault difference in this case was not found).
* If purely the Firecracker Golang SDK is used to load the VM from the snapshot (avoiding firecracker-containerd), the
  nginx container works fine, but the Golang server container still crashes.
* When a second VM is attempted to be loaded from a snapshot using the approach described in the previous bullet, the
  Firecracker vsock backend returns an “Address in use” error, though apparently the first firecracker VM is gracefully
  shut down and all of its network resources are cleaned up and no sockets related to Firecracker could be found in the
  system.

##### Debugging facilities

A custom Firecracker VM rootfs with an SSH server can be generated using this
[debug branch](https://github.com/vhive-serverless/firecracker-containerd/tree/firecracker-v1.4.1-vhive-integration-debug)
by following the instructions from firecracker-containerd's image builder tool's
[README](https://github.com/firecracker-microvm/firecracker-containerd/blob/main/tools/image-builder/README.md#generation).
The VM can be then accessed knowing its IP using the
[firecracker_rsa](https://github.com/vhive-serverless/firecracker-containerd/blob/firecracker-v1.4.1-vhive-integration-debug/tools/image-builder/firecracker_rsa)
private key via `ssh -i ./firecracker_rsa root@<VM IP address>`. This allows, for instance, to attach to the running
container process via GDB (this opportunity was not yet explored).

The container disk state can be explored using `e2fsck`.

The Firecracker VM rootfs can potentially be extended with other debugging facilities (for instance, `iotop` could help
tracking page loading operations).

---

## Incompatibilities and limitations

### Firecracker device renaming

When a snapshot of a VM created by firecracker-containerd is restored, due to the non-deterministic container snapshot
path (it depends on the containerd snapshotter implementation), the container snapshot path at the time of the snapshot
creation is different from the container snapshot path at the time of the snapshot loading.

Currently, the Firecracker snapshotting API does not support device renaming
[firecracker-microvm/firecracker#4014](https://github.com/firecracker-microvm/firecracker/issues/4014), so in order to
overcome
this limitation we need to maintain patches
to [Firecracker](https://github.com/vhive-serverless/firecracker/tree/v1.4.1-vhive-integration)
and
the [Firecracker Golang SDK](https://github.com/vhive-serverless/firecracker-go-sdk/tree/firecracker-v1.4.1-vhive-integration)
that manually substitute the VM state with the path of the block device backing the container snapshot to the path of
the new container snapshot path received from the `LoadSnapshot` request of the Firecracker snapshotting API.

### Snapshot filesystem changes capture and restoration

Currently, the filesystem changes are captured in a “patch file”, which is created by mounting both the original
container image and the VM block device and extracting the changes between both using rsync. Even though rsync
uses some optimisations such as using timestamps and file sizes to limit the amount of reads, this procedure is quite
inefficient and could be sped up by directly extracting the changed block offsets from the thinpool metadata device
and directly reading these blocks from the VM rootfs block device. These extracted blocks could then be written
back at the correct offsets on top of the base image block device to create a root filesystem for the to be restored
VM. However, for this approach to work across nodes for remote snapshots, support to [deterministically flatten a
container image into a filesystem](https://assets.amazon.science/25/06/d2e5ea9c411c9e4d366aa2fbbca5/on-demand-container-loading-in-aws-lambda.pdf)
(GH-824) would be required to ensure the block devices of identical images pulled to different nodes are bit-identical.
In addition, further optimisations would be necessary to more efficiently extract filesystem changes from the thinpool
metadata device.

### Performance limitations

Currently, snapshots require a new block device and network device with the exact state of the snapshotted VM to be
created before restoring the snapshot. The network namespace and `devmapper` block device creation turn out to be a
bottleneck when concurrently restoring many snapshots. Approaches that reduce the impact of these operations could
further speedup the VM snapshot restore latency at high load.

### UPF snapshot compatibility

Currently, the vanilla Firecracker snapshot functionality is currently not integrated (GH-807) with the
[Record-and-Prefetch (REAP)](papers/REAP_ASPLOS21.pdf) accelerated snapshots and thus cannot be used in combination with
the `-upf` flag. The UPF snapshots are available on
a [legacy branch](https://github.com/vhive-serverless/vHive/tree/legacy-firecracker-v0.24.0-with-upf-support).