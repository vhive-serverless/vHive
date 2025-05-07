#!/bin/bash

set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT="$( cd $DIR && cd ../.. && pwd)"
BINS=$ROOT/bin
CONFIGS=$ROOT/configs/demux-snapshotter

sudo mkdir -p /etc/demux-snapshotter/

cd $ROOT

sudo cp $CONFIGS/config.toml /etc/demux-snapshotter/

sudo mkdir -p /var/lib/demux-snapshotter

DST=/usr/local/bin

for BINARY in demux-snapshotter http-address-resolver
do
  [ -f $DST/$BINARY ] || sudo cp $BINS/$BINARY $DST/
done
