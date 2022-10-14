#!/bin/bash

function taint_workers() {
  kubectl get nodes > tmp
  sed -i '1d' tmp

  while read LINE; do
      NODE=$(echo $LINE | cut -d ' ' -f 1)
      TYPE=$(echo $LINE | cut -d ' ' -f 3)

      if [[ $TYPE != *"master"* ]]; then
          kubectl taint nodes ${NODE} key1=value1:NoSchedule
      fi
  done < tmp

  rm tmp
}

function untaint_workers() {
  kubectl get nodes > tmp
  sed -i '1d' tmp

  while read LINE; do
      NODE=$(echo $LINE | cut -d ' ' -f 1)
      TYPE=$(echo $LINE | cut -d ' ' -f 3)

      if [[ $TYPE != *"master"* ]]; then
          kubectl taint nodes ${NODE} key1-
      fi
  done < tmp

  rm tmp
}