#!/bin/bash
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT="$( cd $DIR && cd .. && cd .. && pwd)"
SCRIPTS=$ROOT/scripts

$SCRIPTS/setup_system.sh
$SCRIPTS/setup_containerd.sh

$SCRIPTS/install_stock.sh
$SCRIPTS/create_devmapper.sh

