#!/bin/bash

sudo setfacl -m u:${USER}:rw /dev/kvm
sudo modprobe msr

echo Killing the snmp daemon...
sudo service snmpd stop

MY_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

echo Set CPU frequency if bios supports it
#sudo apt-get install linux-tools-$(uname -r)
#sudo cpupower frequency-set -f 2500000

echo Disabled THP
sudo sh -c "echo never > /sys/kernel/mm/transparent_hugepage/enabled"
sudo sh -c "echo never > /sys/kernel/mm/transparent_hugepage/defrag"
echo Disabled NUMA balancing
sudo sh -c "echo 0 > /proc/sys/kernel/numa_balancing"

echo Disabled irqbalance daemon
#MASK='c000,00c00000' # 22,46,23,47 cores; change the bit mask accordingly
#$MY_DIR/irq_balance.sh eno1 $MASK
#IFACE=eth0
#$MY_DIR/show_irq_affinity.sh $IFACE

echo System is stable now
echo WARNING: Make sure to set kernel boot args: usbcore.autosuspend=-1 intel_pstate=disable intel_iommu=on iommu=pt nokaslr rhgb quiet tsc=reliable cpuidle.off=1 idle=poll intel_idle.max_cstate=0 processor.max_cstate=0 pcie_aspm=off processor.ignore_ppc=1
echo Check /proc/cmdline contents:
cat /proc/cmdline

echo Switch off swapping
sudo swapoff --all

echo ====== CPU overcommitment settings ======
# Load kernel module
sudo modprobe kvm_intel
# Configure packet forwarding
#sudo sysctl -w net.ipv4.conf.all.forwarding=1
# Avoid "neighbour: arp_cache: neighbor table overflow!"
#sudo sysctl -w net.ipv4.neigh.default.gc_thresh1=1024
#sudo sysctl -w net.ipv4.neigh.default.gc_thresh2=2048
#sudo sysctl -w net.ipv4.neigh.default.gc_thresh3=4096
#sudo sysctl -w net.ipv4.ip_local_port_range="32769 65535"

#sudo sysctl -w kernel.pid_max=4194303
#sudo sysctl -w kernel.threads-max=999999999