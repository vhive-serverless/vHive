
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


# Validation of network configuration of reloaded uVM
To check the configuration, from vHive logs, get the lines of log that document the network configuration 
for the uVM you want to debug. It should look something like so:
```
time="2023-01-19T23:19:27.830614078-07:00" level=debug msg="Allocated a new network config" CloneIP=172.18.0.53 ContainerCIDR=172.16.0.2/24 ContainerIP=172.16.0.2 GatewayIP=172.16.0.1 HostDevName=tap0 NamespaceName=uvmns52 NamespacePath=/var/run/netns/uvmns52 Veth0CIDR=172.17.0.210/30 Veth0Name=veth52-0 Veth1CIDR=172.17.0.209/30 Veth1Name=veth52-1 funcID=4
time="2023-01-19T23:19:27.837626054-07:00" level=debug msg="Creating tap for virtual network" IP gateway=172.16.0.1/24 namespace=uvmns53 tap=tap0
```

Then run ./check_network.sh (give execution rights if you can't run it), after replacing the environment variables accordingly, and validate that the returned network configuration matches vHive logs.
