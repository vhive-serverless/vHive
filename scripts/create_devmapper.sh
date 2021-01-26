#!/bin/bash

sudo mkdir -p /var/lib/firecracker-containerd/snapshotter/devmapper

pushd /var/lib/firecracker-containerd/snapshotter/devmapper > /dev/null
DIR=/var/lib/firecracker-containerd/snapshotter/devmapper
POOL=fc-dev-thinpool

# When executed inside a docker container, this command returns the container ID of the container.
# on a non container environment, this returns "/".
CONTAINERID=$(basename $(cat /proc/1/cpuset))

# Docker container ID is 64 characters long.
if [ 64 -eq ${#CONTAINERID} ]; then
    POOL="${CONTAINERID}_thinpool"
fi

if [[ ! -f "${DIR}/data" ]]; then
    sudo touch "${DIR}/data"
    sudo truncate -s 100G "${DIR}/data"
fi

if [[ ! -f "${DIR}/metadata" ]]; then
    sudo touch "${DIR}/metadata"
    sudo truncate -s 2G "${DIR}/metadata"
fi

DATADEV="$(sudo losetup --output NAME --noheadings --associated ${DIR}/data)"
if [[ -z "${DATADEV}" ]]; then
    DATADEV="$(sudo losetup --find --show ${DIR}/data)"
fi

METADEV="$(sudo losetup --output NAME --noheadings --associated ${DIR}/metadata)"
if [[ -z "${METADEV}" ]]; then
    METADEV="$(sudo losetup --find --show ${DIR}/metadata)"
fi

SECTORSIZE=512
DATASIZE="$(sudo blockdev --getsize64 -q ${DATADEV})"
LENGTH_SECTORS=$(bc <<< "${DATASIZE}/${SECTORSIZE}")
DATA_BLOCK_SIZE=128
LOW_WATER_MARK=32768
THINP_TABLE="0 ${LENGTH_SECTORS} thin-pool ${METADEV} ${DATADEV} ${DATA_BLOCK_SIZE} ${LOW_WATER_MARK} 1 skip_block_zeroing"
echo "${THINP_TABLE}"

if ! $(sudo dmsetup reload "${POOL}" --table "${THINP_TABLE}"); then
    sudo dmsetup create "${POOL}" --table "${THINP_TABLE}"
fi

popd > /dev/null
