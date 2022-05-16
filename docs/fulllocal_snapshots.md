# vHive fulllocal snapshots guide

The default snapshots in vHive use an offloading based technique that leaves the shim and other resources running upon shutting down a VM such that it can be re-used in the future. This technique has the advantage that a shim does not have to be recreated and the block and network devices of the previously stopped VM can be reused. This approach does however limit the amount of VMs that can be booted from a snapshot to the amount of VMs that have been offloaded. An alternative approach is to allow loading an arbitrary amount of VMs from a single snapshot by creating a new shim, block and network devices upon loading a snapshot. This functionality can be enabled by running vHive using the `-snapshots -fulllocal` flags. Additionally, the following flags can be used to further configure the fullLocal snapshots

* `-isSparseSnaps`: store the memory file as a sparse file to make the storage size closer to the actual memory utilized by the VM, rather than the memory allocated to the VM
* `-snapsStorageSize [capacityGiB]`: specify the amount of capacity that can be used to store snapshots
* `-netPoolSize [capacity]`: keep around a pool of [capacity] network devices that can be used by VMs to keep network creation off the cold start path
