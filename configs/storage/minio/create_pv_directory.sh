#!/bin/bash

sudo rm -rf /mnt/data
sudo mkdir -p /mnt/data
sudo chmod 777 /mnt/data

# Unmount the XFS filesystem
sudo rm /mnt/tmpfs/ramdisk.img
sudo umount -f /mnt/tmpfs
sudo umount -f /mnt/ramdisk
sudo losetup -d /dev/loop0
sudo rm -rf /mnt/ramdisk
sudo rm -rf /mnt/tmpfs

# Create and mount a 24G RAM disk
sudo mkdir -p /mnt/tmpfs
sudo mount -t tmpfs -o size=24G tmpfs /mnt/tmpfs
sudo dd if=/dev/zero of=/mnt/tmpfs/ramdisk.img bs=1M count=24576
sudo losetup /dev/loop0 /mnt/tmpfs/ramdisk.img
sudo mkfs.xfs /dev/loop0
sudo mkdir -p /mnt/ramdisk
sudo mount -t xfs /dev/loop0 /mnt/ramdisk
df -h /mnt/ramdisk

