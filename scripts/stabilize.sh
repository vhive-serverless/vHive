#!/bin/bash

set -e

pushd $HOME >> /dev/null

# DOCKER
sudo apt-get update

sudo apt-get -y install \
	apt-transport-https \
	ca-certificates \
	curl \
	gnupg-agent \
	software-properties-common

curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -

sudo apt-key fingerprint 0EBFCD88

sudo add-apt-repository \
   "deb [arch=amd64] https://download.docker.com/linux/ubuntu \
   $(lsb_release -cs) \
   stable"

sudo apt-get -y install docker-ce docker-ce-cli containerd.io

sudo docker run hello-world

# GO
wget https://golang.org/dl/go1.14.6.linux-amd64.tar.gz

tar -C /usr/local -xzf go1.14.6.linux-amd64.tar.gz

export PATH=$PATH:/usr/local/go/bin

echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile

# use ssh for git, set up GOPRIVATE
git config --global url."git@github.com:".insteadOf https://github.com/
go env -w GOPRIVATE=github.com/ustiugov/*

# stack size, # of open files, # of pids
echo "* soft nofile 1000000" >> /etc/security/limits.conf
echo "* hard nofile 1000000" >> /etc/security/limits.conf
echo "root soft nofile 1000000" >> /etc/security/limits.conf
echo "root hard nofile 1000000" >> /etc/security/limits.conf
echo "* soft nproc 4000000" >> /etc/security/limits.conf
echo "* hard nproc 4000000" >> /etc/security/limits.conf
echo "root soft nproc 4000000" >> /etc/security/limits.conf
echo "root hard nproc 4000000" >> /etc/security/limits.conf
echo "* soft stack 65536" >> /etc/security/limits.conf
echo "* hard stack 65536" >> /etc/security/limits.conf
echo "root soft stack 65536" >> /etc/security/limits.conf
echo "root hard stack 65536" >> /etc/security/limits.conf

popd >> /dev/null

