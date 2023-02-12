#!/bin/bash

# create log file
rm -rf log_image_pulls.txt
touch log_image_pulls.txt

sudo docker image rm ghcr.io/ease-lab/helloworld:var_workload
sudo docker image rm ghcr.io/ease-lab/pyaes:var_workload
sudo docker image rm vhiveease/rnn_serving:var_workload

for i in {1..100}
do
	echo -e "helloworld pull  $i: \n" >> log_image_pulls.txt
	time sudo docker pull ghcr.io/ease-lab/helloworld:var_workload >> log_image_pulls.txt
	echo -e "pyaes pull  $i: \n"  >> log_image_pulls.txt
	time sudo docker pull ghcr.io/ease-lab/pyaes:var_workload >> log_image_pulls.txt
  echo -e "rnn pull  $i: \n" >> log_image_pulls.txt
	time sudo docker pull vhiveease/rnn_serving:var_workload >> log_image_pulls.txt

	# remove images
	sudo docker image rm ghcr.io/ease-lab/helloworld:var_workload
	sudo docker image rm ghcr.io/ease-lab/pyaes:var_workload
	sudo docker image rm vhiveease/rnn_serving:var_workload
done