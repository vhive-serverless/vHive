#!/bin/bash

VM_NUM=$1

if [ $VM_NUM -eq 1 ]
then
DELAY=35000
RPS=5
elif [ $VM_NUM -eq 4 ]
then
DELAY=70000
RPS=12
elif [ $VM_NUM -eq 10 ]
then
DELAY=140900
RPS=50
else
echo -e "VM_NUM argument invalid\n"
exit
fi

sudo mkdir -m777 -p /tmp/ctrd-logs && sudo env "PATH=$PATH" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>/tmp/ctrd-logs/fccd_orch_noupf_log_bench.out 2>/tmp/ctrd-logs/fccd_orch_noupf_log_bench.err &

perf stat -D $DELAY -a -e instructions,LLC-loads,LLC-load-misses,LLC-stores,LLC-store-misses --output perf-${VM_NUM}VMs-rnn_serving.profile sudo env "PATH=$PATH" go test -v -run TestBenchRequestPerSecond -args -vm $VM_NUM -requestPerSec $RPS -executionTime 1 -functions rnn_serving

./scripts/clean_fcctr.sh

echo -e "\n### Report Perf Profile ###\n"
cat perf-${VM_NUM}VMs-lr_training.profile