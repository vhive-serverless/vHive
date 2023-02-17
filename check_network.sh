#!/bin/bash

NET_NAMESPACE_NAME="uvmns52"
TUNNEL_VM_END="veth52-0"
TUNNEL_HOST_END="veth52-1"
CLONE_IP="172.18.0.53"
VETH_PAIR="veth52"

echo -e "net namespace\n" && ip netns list | grep $NET_NAMESPACE_NAME && \
echo -e "\n\n" && \
echo -e "tap0 set up:\n" && sudo ip netns exec $NET_NAMESPACE_NAME ip address show dev tap0  && \
echo -e "\n\n" && \
echo -e "show veth pair:\n" && sudo ip netns exec $NET_NAMESPACE_NAME ip -c link show type veth && \
echo -e "\n\n" && \
echo -e "show veth-0 uvm side:\n" && sudo ip netns exec $NET_NAMESPACE_NAME sudo ip address show dev $TUNNEL_VM_END && \
echo -e "\n\n" && \
echo -e "routing rules in uvm space:" && sudo ip netns exec $NET_NAMESPACE_NAME sudo ip route show && \
echo -e "\n\n" && \
echo -e "nat translation rules source dest to clone ip:" && sudo ip netns exec $NET_NAMESPACE_NAME nft list ruleset && \
echo -e "\n\n" && \
echo -e "show veth-1 host side:" && sudo ip address show dev $TUNNEL_HOST_END && \
echo -e "\n\n" && \
echo -e "show veth-1 host side:" && sudo ip -c link show type veth | grep "$TUNNEL_HOST_END" && \
echo -e "\n\n" && \
echo -e "show clone ip routing rules:" && sudo routel | grep $CLONE_IP && \
echo -e "\n\n" && \
echo -e "forward rule in and out of host int (eno49):" && sudo nft list table filter > filter_rules && grep $VETH_PAIR filter_rules