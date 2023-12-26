#!/bin/bash

echo "PermitRootLogin=yes" | sudo tee -a /etc/ssh/sshd_config

sudo apt-get update -qq
sudo apt-get install -qq -y openssh-server 
sudo service ssh start
eval "$(ssh-agent -s)"

ssh-keygen -t rsa -b 4096 -f ~/.ssh/id_rsa -N "" > /dev/null
cat > ~/.ssh/config <<EOF
  Host host.example
  User runner
  HostName localhost
  IdentityFile ~/.ssh/id_rsa
EOF

# add public key 
cat - ~/.ssh/id_rsa.pub > ~/.ssh/authorized_keys

sudo service ssh restart

# add private key to ssh agent
ssh-add ~/.ssh/id_rsa 

chmod og-rw ~/.ssh

go test -timeout 5m
