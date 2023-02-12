#!/bin/bash

#add macro for port if remote nodes not on CloudLab
PATH_TO_PRIVATE_KEY="/.ssh/id_rsa/id_ed25519"
# Change paths to files depending on username (CloudLab)
USERNAME="dummyUserName"
HOST_MASTER="hp179.utah.cloudlab.us"
HOST_WORKER_STORAGE="hp181.utah.cloudlab.us"



ssh -o 'StrictHostKeyChecking no' -i $PATH_TO_PRIVATE_KEY -p 22 $USERNAME@$HOST_WORKER_STORAGE << EOF
cd /users/estellan/vhive
sudo mkdir -p minio_storage
cd ./configs/storage/minio
EOF


ssh -o 'StrictHostKeyChecking no' -i $PATH_TO_PRIVATE_KEY -p 22 $USERNAME@$HOST_MASTER << EOF
MINIO_NODE_NAME=node-2.vhive-clems.faas-sched-pg0.utah.cloudlab.us MINIO_PATH=/users/estellan/vhive/minio_storage/ envsubst < /users/estellan/vhive/configs/storage/minio/pv.yaml | kubectl apply -f -
sleep 5
kubectl apply -f /users/estellan/vhive/configs/storage/minio/pv-claim.yaml
sleep 5
kubectl apply -f /users/estellan/vhive/configs/storage/minio/deployment.yaml
sleep 5
kubectl apply -f /users/estellan/vhive/configs/storage/minio/service.yaml
sleep 5
EOF




ssh -o 'StrictHostKeyChecking no' -i $PATH_TO_PRIVATE_KEY -p 22 $USERNAME@$HOST_WORKER_STORAGE << EOF
cd /users/estellan/vhive/configs/storage/minio
wget https://dl.min.io/server/minio/release/linux-amd64/minio
chmod +x minio
sudo cp minio /usr/local/bin
wget https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x mc
sudo cp mc /usr/local/bin

mc alias set myminio http://10.96.0.46:9000 minio minio123
mc mb myminio/mybucket
mc anonymous set public myminio/mybucket
EOF