#!/bin/sh

errorExit() {
    echo "*** $*" 1>&2
    exit 1
}

curl --silent --max-time 2 --insecure https://localhost:8443/ -o /dev/null || errorExit "Error GET https://localhost:8443/"
if ip addr | grep -q 10.0.1.254; then
    curl --silent --max-time 2 --insecure https://10.0.1.254:8443/ -o /dev/null || errorExit "Error GET https://10.0.1.254:8443/"
fi