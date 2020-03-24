#!/bin/bash

NI_NUM=$1

ECHO="echo "
ECHO=""

echo ========== Cleaning old taps and bridges ===========

COUNT=`ls /sys/class/net/ | wc -l`

MY_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

upperlim=$COUNT
parallel=10

for ((i=0; i<parallel; i++)); do
  s=$((i * upperlim / parallel))
  e=$(((i+1) * upperlim / parallel))
  for ((j=s; j<e; j++)); do
    $ECHO sudo ip link del fc-$j-tap0 2> /dev/null
  done &
done

wait

$ECHO sudo ip link del br6 2> /dev/null
$ECHO sudo ip link del br7 2> /dev/null


echo ========== Creating new taps and bridges ===========

$ECHO sudo ip link add br6 type bridge
$ECHO sudo ip link add br7 type bridge


upperlim=$NI_NUM
parallel=10

for ((i=0; i<parallel; i++)); do
  s=$((i * upperlim / parallel))
  e=$(((i+1) * upperlim / parallel))
  for ((j=s; j<e; j++)); do
    TAP=fc-$j-tap0
    BRIDGE=br$((j % 2 + 6))
    $ECHO sudo ip tuntap add $TAP mode tap
    $ECHO sudo ip link set $TAP master $BRIDGE
    $ECHO sudo ip link set dev $TAP up
  done &
done

wait

$ECHO sudo ip link set dev br6 up
$ECHO sudo ip addr add dev br6 196.128.0.1/10
$ECHO sudo ip link set dev br7 up
$ECHO sudo ip addr add dev br7 197.128.0.1/10
