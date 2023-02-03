#!/bin/bash

# create log file
rm -rf log_master_script.txt
rm -rf ./latencies
mkdir latencies
touch log_master_script.txt

echo -e "Deploy function\n"
source /etc/profile >> log_master_script.txt && pushd ./examples/deployer >> log_master_script.txt && go build >> log_master_script.txt && popd && ./examples/deployer/deployer >> log_master_script.txt
echo -e "Deploy function ended\n"


for i in {1..200}
do

	ts=$(date +%s%N)
	echo -e "Invoke function run $i START\n" >> log_master_script.txt

	pushd ./examples/invoker >> log_master_script.txt && go build >> log_master_script.txt && popd && ./examples/invoker/invoker >> log_master_script.txt


	echo -e "Invoke function run $i END at time: $((($(date +%s%N) - $ts)/1000000))\n" >> log_master_script.txt

	now=$(date +"%T")
	echo -e "Sleep for 1 minute: $now\n" >> log_master_script.txt
	sleep 70

	now=$(date +"%T")
	echo -e "Sleep ended at time run $i: $now\n" >> log_master_script.txt

	touch ./latencies/rps$i.00_lat.csv
	cat rps1.00_lat.csv >> ./latencies/rps$i.00_lat.csv
	echo -e "Check if pods still up\n" >> log_master_script.txt
	kubectl get pods -A | head -n 5 >> log_master_script.txt
done