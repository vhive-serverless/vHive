
## Prerequisites
Run the scripts/setup_minio.sh script after modifying the macros on your local machine. This will setup Minio global storage in the cluster.

## Remote snapshots (in progress prototype)

To run, use -isRemoteSnap flag when executing vHive as so:

````
./vhive -dbg -snapshots -fulllocal -isRemoteSnap
````

vHive will try to reload from snapshot only if it will have the mem, snap, patch files in its directory; These paths are hardcoded for now, so make sure to change them in cri/firecracker/coordinator.go lines 208, 211-213.

## Note
The upload, download to minio has been commented out as the restoration from 
snapshot is not yet working and the PUT/GET operations were too time intensive
for debugging scenarios. The following have been commented out:
- cri/firecracker/coordinator.go: lines 125-207 (check for existing snapshot in global storage for the requested function)
- ctriface/iface.go: lines 518-593 (upload snap files to Minio; minio details need to be extracted in macros, code needs
refactoring here and the files for the snapshot and details for minio bucket need changing)

 
Exception for:
- ctriface/iface.go: lines 288 - 311; 334-340 (destruction of uVM when reloaded from remote snapshot; this part either leads to errors 
zombie state of containers, but since the state from the snapshot is not successfully reloaded, it is unclear where the errors 
from this stage come from)


# Manual reload from remote snapshot
See ./manual_reload to reload a serverless function from a remote snapshot without going through vHive, and using firecracker-containerd directly.