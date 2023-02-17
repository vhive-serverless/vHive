## Collecting

Measurements for function reloading times and QPS latencies ca be collected using the ./scripts/benchmark_full_vs_default_snaps/master_script.sh which generates a log text file with latencies and a ./latencies folder in which all the latencies for responses for all invocation runs will be saved.Details about function creation times are collected from vhive log files (see https://github.com/vhive-serverless/vHive/pull/632 on how to extract all logs from vHive).
Measurements for function image pull latencies were gathered with ./scripts/benchmark_full_vs_default_snaps/bench_images.sh.

To collect snapshot file download latencies, make sure you first run from your local machine the scripts/benchmark_full_vs_default_snaps/set_minio.sh script to set up minio. Then, on the cluster node, ensure you have the snapshot files in the hardcoded path from scripts/benchmark_full_vs_default_snaps/minioFput.go file or change de path to reflect teh location of the snapshotted files. You need the snapshot files for all 3 following functions: helloworld, rnn, pyaes, which are knative workloads offered in the vHive quickstart. Then run scripts/benchmark_full_vs_default_snaps/bench_snap_files.sh after running scripts/benchmark_full_vs_default_snaps/minioFput.go and modify it for each function image.


## Processing
Manual raw processing via bash commands/oneliners or processing scripts: ./scripts/benchmark_full_vs_default_snaps/process_latencies.py.

## Plot generation
Generate plots via the follwoing scripts in the following subfolders of ./scripts/benchmark_full_vs_default_snaps/: avg_reload_time_from_snap, 90th_percentile_response_times, prereqs_for_snap_restoration.
The scripts use dicstionaries of values which come from the previous, processing step. Each script has comments inside describing the expected data and the plot to be generated.