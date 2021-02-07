#!/bin/bash

set -e

sudo apt-get install linux-tools-`uname -r` -y

git clone https://github.com/andikleen/pmu-tools /usr/local/pmu-tools

sudo sh -c  "echo 'export PATH=\$PATH:/usr/local/pmu-tools' >> /etc/profile"
sudo sh -c  "echo 'echo 0 > /proc/sys/kernel/nmi_watchdog' >> /etc/profile"

# first run, download essential files
toplev sleep 1 >> /dev/null