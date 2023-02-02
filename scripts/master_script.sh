#!/bin/bash

# create log file
rm -rf log_master_script.txt
touch log_master_script.txt


now=$(date +"%T")
echo "Invoke function run i START at time: $now" >> log_master_script.txt

source /etc/profile && pushd ./examples/deployer && go build && popd && ./examples/deployer/deployer

now=$(date +"%T")
echo "Invoke function run i END at time: $now" >> log_master_script.txt

echo "Sleep for 1 minute" >> log_master_script.txt
sleep 60

now=$(date +"%T")
echo "Sleep ended at time: $now" >> log_master_script.txt