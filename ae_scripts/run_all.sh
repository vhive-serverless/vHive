# MIT License
#
# Copyright (c) 2020 Dmitrii Ustiugov, Plamen Petrov and EASE lab
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

#!/bin/bash

set -e

# Kill all children of the process upon a keyboard interrupt or exit
trap "exit" INT TERM
trap "kill 0" EXIT

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

ROOT="$( cd $DIR && cd .. && pwd)"

source /etc/profile
GO_BIN=`which go`

cd $ROOT

############## Supplementary functions below ##############
die () {
    echo >&2 "$@"
    exit 1
}

################### Main body below #######################

[ "$#" -eq 1 ] || die "1 argument required, $# provided"

mode=$1

if [[ ! $mode =~ ^(baseline|reap)$ ]]; then
    die "Wrong mode specified, the scripts supports only the following modes: baseline, reap"
fi

host_ip=`curl ifconfig.me`

#wlds=(helloworld chameleon pyaes image_rotate_s3 json_serdes_s3 lr_serving cnn_serving rnn_serving lr_training_s3 video_processing_s3)

# Reading the file with functions
i=0
for j in `cat $ROOT/ae_scripts/functions.txt`
do
    wlds[$i]=$j
    i=$(($i+1))
done

echo The experiment will run the following functions: ${wlds[@]}

all_results_path=all_results
results_path=$all_results_path/$mode
rm -rf $results_path || echo Folder $results_path exists, removing the old one.
mkdir -p $results_path

echo Run MinIO server as a daemon
sudo pkill -9 minio || echo
$ROOT/function-images/minio_scripts/start_minio_server.sh 1>/dev/null &
sleep 1

echo ======================================================
echo ============== Starting the experiment ===============
echo ======================================================

for wld in "${wlds[@]}"
do
    echo
    echo About to run $mode/$wld experiment in $mode mode

    if [[ "$mode" == "reap" ]]; then
        modeFlag=-upfTest
    fi

    ############ Clean up after previous experiment ########
    echo Killing the containerd daemon and cleaning up.
    ./scripts/clean_fcctr.sh 1>clean.out 2>clean.err
    wld_dir=$results_path/$wld
    mkdir -p $wld_dir

    ########################################################
    echo Starting containerd daemon
    sudo PATH=$PATH /usr/local/bin/firecracker-containerd \
        --config /etc/firecracker-containerd/config.toml \
        1>$wld_dir/containerd.out 2>$wld_dir/containerd.err &

    echo Wait for containerd to start
    sleep 2

    ########################################################
    echo Running the actual benchmark... may take up to one minute.

    sudo $GO_BIN test -v -run TestBenchServe \
        -args -iter 5 \
        -snapshotsTest \
        -benchDirTest $results_path/$wld \
        -metricsTest \
        -funcName $wld \
        -minioAddress $host_ip:9000 \
        $modeFlag 1>$wld_dir/test.out 2>$wld_dir/test.err
    
    sleep 1

    ################# Process latency stats #################
    addInstanceMetric=`cat $wld_dir/serve.csv | tail -n 1 | cut -d"," -f2`
    if [[ "$mode" == "baseline" ]]; then
        funcInvocationMetric=`cat $wld_dir/serve.csv | tail -n 1 | cut -d"," -f8`
    else
        funcInvocationMetric=`cat $wld_dir/serve.csv | tail -n 1 | cut -d"," -f10`
    fi

    totalMetric=$(( (addInstanceMetric + funcInvocationMetric)/1000 ))

    echo Average function invocation took $totalMetric milliseconds

    echo $wld,$mode,$totalMetric >> $all_results_path/results.csv
done

sudo pkill -9 minio

echo ======================================================
echo ====================== All done! =====================
echo ======================================================
