# vHive profiling tool

## Methodology

The tool loads certain amount of requests per second (RPS) to vHive for a time duration. 

The load procedure is splitted into different steps from 5% to 100% of input RPS. If the tail latency at a load step violates the contraint which is 20x image service time, the tool will return the profiling results one step before tail latency violation.

A load step is then divided into three parts: warm-up, profile and cool-down. During the profile period, the tool records the average execution time of invocations and how many invocations are returned successfully. It also invokes [pmu-tools](https://github.com/andikleen/pmu-tools) to monitor user-defined nodes followed by [TopDown method](https://ieeexplore.ieee.org/document/6844459). After the measurement is finished, successful RPS per core, average execution time and the average counters of nodes will be saved in the `profile.csv`. 

## Profile fixed number of VMs and RPS

Profile a single VM with `helloworld` image and 20 RPS at TopDown level 1 :
```
sudo env "PATH=$PATH" go test -v -timeout 99999s -run TestProfileSingleConfiguration -args -funcNames helloworld -vm 1 -rps 20 -l 1
```
bottleneck nodes will be printed out with their value. 
    
To drill down on bottlenecks, profile the same configuration with specific nodes. For example, the bottleneck is in the frontend at level 1 and now we want to profile level 2 of frontend:
```
sudo env "PATH=$PATH" go test -v -timeout 99999s -run TestProfileSingleConfiguration -args -funcNames helloworld -vm 1 -rps 20 -l 1 -nodes '!+Frontend_Bound*/2,+MUX'
```

## Profile incrementing VM number

Profile from 4 VMs to 32 VMs (increment step is 4) with `helloworld` image at TopDown level 1:
```
sudo env "PATH=$PATH" go test -v -timeout 99999s -run TestProfileIncrementConfiguration -args -funcNames cnn_serving -vmIncrStep 4 -maxVMNum 32 -l 1
```
Once the profile iteration is finished, all results will be saved in the `profile.csv`. Then, plotter retrives the contents in the file and plots according to the attributes.
