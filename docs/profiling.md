# vHive profiling tool

## Methodology

The tool is for benchmarking vHive instances by recording hardware counters, requests per second
 (RPS) and execution time for functions running inside instances.

The tool includes following components: 
- A loader function loads requests to vHive round by round (increment 5% of input RPS each round 
  by default) until input RPS and profiles performance. if tail latency violates 10x image 
  unloaded service time at a step, the function returns the metric before it.
- A single latency measurement goroutine mimics a rare event loader that injects a request least 
  every 500ms and measures runtimes of requests to compute mean latency and 90 percentile latency.
- A bind socket function binds all VMs to socket 1 (only compatible with a 2x 16-core machine or 
  above).
- A profiler invokes [toplev](https://github.com/andikleen/pmu-tools) to collect hardware counters.
- A plotter plots line charts for a list of metrics.

A load step divides into three parts: warm-up, profiling and cool-down. During the profile 
period, the benchmark records the average execution time of invocations and how many invocations 
return successfully. During the profile period, it also invokes [pmu-tools](https://github.com/andikleen/pmu-tools) 
to profile user-defined counters followed by [TopDown method](https://ieeexplore.ieee.org/document/6844459). 
After the measurement finishes, completed RPS per core, average execution time and the average 
counters will save in the `profile.csv`.

## Runtime Arguments
```
General:
-warmUpTime   FLOAT  The warm up time before profiling in seconds
-profileTime  FLOAT  The profiling time in seconds
-coolDownTime FLOAT  The cool down time after profiling in seconds
-loadStep     FLOAT  The percentage of target RPS the benchmark loads at every step
-funcNames    STR    Names of the functions to benchmark, separated by comma
-bindSocket   BOOL   Bind all VMs to socket 1 and profile one physical core only
                     (only compatible with a 2x 16-core machine or above)

TestProfileSingleConfiguration:
-vm           INT    The number of VMs
-rps          INT    The target requests per second

TestProfileIncrementConfiguration:
-vmIncrStep   INT    The increment VM number
-maxVMNum     INT    The maximum VM number

Profiler:
-l            INT    Profile level
-I            UINT   Print count deltas every N milliseconds
-nodes        STR    Include or exclude nodes (with + to add, -|^ to remove, 
                     comma separated list, wildcards allowed, 
                     add * to include all children/siblings, 
                     add /level to specify highest level node to match, 
                     add ^ to match related siblings and metrics, 
                     start with ! to only include specified nodes)
```

## Pre-requisites
Assume you are at the root of this repository, run `scripts/install_pmutool.sh` to install
essential tools for profiling and binding.

## Quick-start guide
Function `TestProfileSingleConfiguration` is for collecting counters from a fixed number of VMs and RPS setting 
during profiling period.

Profile a single VM with `helloworld` image and 20 RPS at TopDown level 1 :
```
sudo env "PATH=$PATH" go test -v -timeout 99999s -run TestProfileSingleConfiguration -args -funcNames helloworld -vm 1 -rps 20 -l 1
```
Bottleneck counters will be printed out with their value at each step during 
execution, such as:
```
...
INFO[] Current RPS: 1200
INFO[] Bottleneck Backend_Bound with value 75.695000
...
```
    
To study microarchitectural bottlenecks in more detail, profile the same configuration with sub-level counters. 
For example, the bottleneck is in the backend bound at level 1 and now we want to profile level 2 of the backend bound:
```
sudo env "PATH=$PATH" go test -v -timeout 99999s -run TestProfileSingleConfiguration -args -funcNames helloworld -vm 1 -rps 20 -nodes '!+Backend_Bound*/2,+MUX'
```

Function `TestProfileIncrementConfiguration` will load the pre-stored number of RPS and 
increment the number of VMs step by step until it reaches the input maximum. At each 
step, this function behaves as same as `TestProfileSingleConfiguration`

Profile from 4 VMs to 32 VMs (increment step is 4) with `helloworld` image at TopDown level 1:
```
sudo env "PATH=$PATH" go test -v -timeout 99999s -run TestProfileIncrementConfiguration -args -funcNames cnn_serving -vmIncrStep 4 -maxVMNum 32 -l 1
```
Once the profiling iteration finishes, all results will save in the `profile.csv`. Then, the plotter retrieves 
the contents in the file and plots according to the attributes.
