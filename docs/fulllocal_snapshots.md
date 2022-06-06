# vHive full local snapshots

When using Firecracker as the sandbox technology in vHive, two snapshotting modes are supported: a default mode and a 
full local mode. The default snapshot mode use an offloading based technique which leaves the shim and other resources 
running upon shutting down a microVM such that it can be re-used in the future. This technique has the advantage that 
the shim does not have to be recreated and the block and network devices of the previously stopped microVM can be 
reused, but limits the amount of microVMs that can be booted from a snapshot to the amount of microVMs that have been 
offloaded. The full local snapshot mode instead allows loading an arbitrary amount of microVMs from a single snapshot. 
This is done by creating a new shim and the required block and network devices upon loading a snapshot and creating an 
extra patch file containing the filesystem differences written by the microVM upon snapshot creation. To enable the 
full local snapshot functionality, vHive must be run with the `-snapshots` and `-fulllocal` flags. In addition, the 
full local snapshot mode can be further configured using the following flags:

- `isSparseSnaps`: store the memory file as a sparse file to make its storage size closer to the actual size of the memory utilized by the microVM, rather than the memory allocated to the microVM
- `snapsStorageSize [capacityGiB]`: specify the amount of capacity that can be used to store snapshots
- `netPoolSize [capacity]`: the amount of network devices in the network pool, which can be used by microVMs to keep the network initialization off the cold start path

## Remote snapshots

Rather than only using the snapshots available locally on a node, snapshots can also be transferred between nodes to 
potentially accelerate cold start times and reduce memory utilization, given that proper mechanisms are in place to 
minimize the snapshot network transfer latency. This could be done by storing snapshots in a global storage solution 
such as S3, or directly distributing snapshots between compute nodes. The full local snapshot functionality in vHive 
can be used to implement such functionality. To implement this, the container image used by the snapshotted microVM 
must be available on the local node where the remote snapshot will be restored. This container image can be used in 
combination with the filesystem changes stored in the snapshot patch file to create a device mapper snapshot that 
contains the root filesystem needed by the restored microVM. After recreating the root filesystem block device, the 
microVM can be created from the fetched memory file and microVM state similarly to how this is done for the full local 
snapshots.

## Incompatibilities and limitations

### Snapshot filesystem changes capture and restoration

Currently, the filesystem changes are captured in a “patch file”, which is created by mounting both the original 
container image and the microVM block device and extracting the changes between both using rsync. Even though rsync 
uses some optimisations such as using timestamps and file sizes to limit the amount of reads, this procedure is quite 
inefficient and could be sped up by directly extracting the changed block offsets from the thinpool metadata device 
and directly reading these blocks from the microVM rootfs block device. These extracted blocks could then be written 
back at the correct offsets on top of the base image block device to create a root filesystem for the to be restored 
microVM. Support for this alternative approach is provided through the `ForkContainerSnap` and `CreateDeviceSnapshot` 
functions. However, for this approach to work across nodes for remote snapshots, support to [deterministically flatten a container image into a filesystem](https://www.youtube.com/watch?v=A-7j0QlGwFk) 
would be required to ensure the block devices of identical images pulled to different nodes are bit identical. 
In addition, further optimizations would be necessary to more efficiently extract filesystem changes from the thinpool 
metadata device rather than current method, which relies on the devicemapper `reserve_metadata_snap` method to create
a snapshot of the current metadata state in combination with `thin_delta` to extract changed blocks.

### Performance limitations

The full local snapshot mode requires a new block device and network device with the exact state of the snapshotted 
microVM to be created before restoring the snapshot. The network namespace and devicemapper block device creation turn 
out to be a bottleneck when concurrently restoring many snapshots. Approaches that reduce the impact of these operations 
could further speedup the microVM snapshot restore latency at high load.

### UPF snapshot compatibility

The full local snapshot functionality is currently not integrated with the [Record-and-Prefetch (REAP)](papers/REAP_ASPLOS21.pdf) 
accelerated snapshots and thus cannot be used in combination with the `-upf` flag.