#!/bin/bash
mkdir -p bin
rm -rf bin/*
GOOS=linux GOARCH=amd64 go build -o bin/easy_openyurt-amd64-linux-0.2.4 .
echo "[Built] 1/2"
GOOS=linux GOARCH=arm64 go build -o bin/easy_openyurt-aarch64-linux-0.2.4 .
echo "[Built] 2/2"
