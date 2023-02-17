#!/bin/bash

# create log file
rm -rf log_pulls.txt
touch log_pulls.txt

# put snapfiles in bucket
go get github.com/minio/minio-go/v7/pkg/credentials && go get github.com/minio/minio-go/v7

# fetch snap files (the go program will print the latencies)
for i in {1..200}
do
	go run minioFget.go >> log_pulls.txt
done