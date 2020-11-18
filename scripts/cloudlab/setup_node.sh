#!/bin/bash
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
SCRIPTS="$( cd $DIR && cd .. && pwd)"

for SC in setup_system.sh install_go.sh setup_containerd.sh create_devmapper.sh
do
    source $SCRIPTS/$SC
done