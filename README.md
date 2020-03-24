# fccd-orchestrator
VM orchestrator for Firecracker-Containerd

# Networking
Taps and bridges need to be created before running the orchestrator with `scripts/create_bridges_taps.sh <NUM>`.
The orchestrator (re)uses the taps (IPs) when starting and stopping VMs.

Note: CNI network configuration is supported in general (commented out for now) but it does not allow to 
keep track of the IP addresses that are given to VMs or reuse them.
