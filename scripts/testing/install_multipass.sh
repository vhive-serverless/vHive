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

# install snap
sudo apt update
sudo apt install snapd
# install multipass
sudo snap install multipass --classic --beta
# install QEMU/KVM
sudo apt-get install -y qemu-kvm libvirt-daemon-system libvirt-clients virt-manager dnsmasq-base qemu bridge-utils
# run virtd
sudo service libvirtd start
sudo update-rc.d libvirtd enable
# set up virtual network
sudo virsh net-destroy default ; sudo virsh net-start default
# set up multipass to use libvirt
snap connect multipass:libvirt
sudo multipass set local.driver=libvirt

# Set up and check KVM
sudo setfacl -m u:${USER}:rw /dev/kvm
[ -r /dev/kvm ] && [ -w /dev/kvm ] && echo "KVM is OK" || echo "KVM setup is wrong"
