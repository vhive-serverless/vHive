[![Build Status](https://travis-ci.com/ustiugov/fccd-orchestrator.svg?token=Dx4z7rB8qLcioVK5Dhsr&branch=master)](https://travis-ci.com/github/ustiugov/fccd-orchestrator)
<br />

# Usage notes

## Networking
The orchestrator creates taps and bridges with IPs when starting/loading and offloading/stopping VMs.

Note 1: CNI network configuration is supported in general (commented out for now) but it does not allow to 
keep track of the IP addresses that are given to VMs or reuse them.

Note 2: When orchestrator panics, it leaves taps and bridges that need to be cleaned manually by running `scripts/clean_fcctr.sh`


# Development notes

## Testing

The orchestrator includes both unit/module tests and end-2-end tests. 

To run the unit tests:
```
make test-subdirs
```

To run the end-to-end tests:
```
make test-orch
```

**Before merging any code, make sure ALL tests pass on your local machine!** Travis testing TBD.
