#!/bin/bash
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT="$( cd $DIR && cd .. && cd .. && pwd)"
SCRIPTS=$ROOT/scripts

source $SCRIPTS/setup_system.sh
source $SCRIPTS/setup_containerd.sh

source $SCRIPTS/cri/install_stock.sh
source $SCRIPTS/create_devmapper.sh

