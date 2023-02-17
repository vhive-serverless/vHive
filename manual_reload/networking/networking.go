package networking

import (
	"fmt"
	"github.com/coreos/go-iptables/iptables"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"io/ioutil"
	"net"
	"regexp"
	"strconv"
)

func createTap(tapName, gatewayIP, netnsName string) error {
	// 1. Create tap device
	la := netlink.NewLinkAttrs()
	la.Name = tapName
	la.Namespace = netnsName
	tap0 := &netlink.Tuntap{LinkAttrs: la, Mode: netlink.TUNTAP_MODE_TAP}
	if err := netlink.LinkAdd(tap0); err != nil {
		return errors.Wrapf(err, "creating tap")
	}

	// 2. Give tap device ip address
	addr, _ := netlink.ParseAddr(gatewayIP)
	addr.Broadcast = net.IPv4(0, 0, 0, 0)
	if err := netlink.AddrAdd(tap0, addr); err != nil {
		return errors.Wrapf(err, "adding tap ip address")
	}

	// 3. Enable tap network interface
	if err := netlink.LinkSetUp(tap0); err != nil {
		return errors.Wrapf(err, "enabling tap")
	}

	return nil
}

func deleteTap(tapName string) error {
	if err := netlink.LinkDel(&netlink.Tuntap{LinkAttrs: netlink.LinkAttrs{Name: tapName}}); err != nil {
		return errors.Wrapf(err, "deleting tap %s", tapName)
	}

	return nil
}

func createVethPair(veth0Name, veth1Name string, veth0NsHandle, veth1NsHandle netns.NsHandle) error {
	veth := &netlink.Veth{netlink.LinkAttrs{Name: veth0Name, Namespace: netlink.NsFd(veth0NsHandle), TxQLen: 1000}, veth1Name, nil, netlink.NsFd(veth1NsHandle)}
	if err := netlink.LinkAdd(veth); err != nil {
		return errors.Wrapf(err, "creating veth pair")
	}

	return nil
}

func deleteVethPair(veth0Name, veth1Name string, veth0NsHandle, veth1NsHandle netns.NsHandle) error {
	if err := netlink.LinkDel(&netlink.Veth{LinkAttrs: netlink.LinkAttrs{Name: veth0Name, Namespace: netlink.NsFd(veth0NsHandle)}, PeerName: veth1Name, PeerNamespace: netlink.NsFd(veth1NsHandle)}); err != nil {
		return errors.Wrapf(err, "deleting veth %s", veth0Name)
	}
	return nil
}

func configVeth(linkName, vethIp string) error {
	// 1. Get link
	veth, err := netlink.LinkByName(linkName)
	if err != nil {
		return errors.Wrapf(err, "Finding veth link")
	}

	// 2. Set IP address
	addr, _ := netlink.ParseAddr(vethIp)
	addr.Broadcast = net.IPv4(0, 0, 0, 0)
	if err := netlink.AddrAdd(veth, addr); err != nil {
		return errors.Wrapf(err, "adding veth link ip address")
	}

	// 3. Enable link
	if err := netlink.LinkSetUp(veth); err != nil {
		return errors.Wrapf(err, "enabling veth link")
	}

	return nil
}

func setDefaultGateway(gatewayIp string) error {
	gw, _, err := net.ParseCIDR(gatewayIp)
	if err != nil {
		return errors.Wrapf(err, "parsing ip")
	}

	defaultRoute := &netlink.Route{
		Dst: nil,
		Gw:  gw,
	}

	if err := netlink.RouteAdd(defaultRoute); err != nil {
		return errors.Wrapf(err, "adding default route")
	}

	return nil
}

func deleteDefaultGateway(gatewayIp string) error {
	gw, _, err := net.ParseCIDR(gatewayIp)
	if err != nil {
		return errors.Wrapf(err, "parsing ip")
	}

	defaultRoute := &netlink.Route{
		Dst: nil,
		Gw:  gw,
	}

	if err := netlink.RouteDel(defaultRoute); err != nil {
		return errors.Wrapf(err, "deleting default route")
	}

	return nil
}

func setupNatRules(vethVmName, hostIp, cloneIp string) error {
	ipt, err := iptables.New()
	if err != nil {
		return errors.Wrapf(err, "creating ip tables")
	}

	// for packets that leave the namespace and have the source IP address of the original guest, rewrite the source
	// address to clone address
	err = ipt.Append("nat", "POSTROUTING", "-o", vethVmName, "-s", hostIp, "-j", "SNAT", "--to", cloneIp, "--wait")
	if err != nil {
		return errors.Wrapf(err, "adding iptable POSTROUTING rule")
	}

	// do the reverse operation; rewrites the destination address of packets heading towards the clone
	// address to source address
	err = ipt.Append("nat", "PREROUTING", "-i", vethVmName, "-d", cloneIp, "-j", "DNAT", "--to", hostIp, "--wait")
	if err != nil {
		return errors.Wrapf(err, "adding iptable POSTROUTING rule")
	}

	return nil
}

func deleteNatRules(vethVmName, hostIp, cloneIp string) error {
	ipt, err := iptables.New()
	if err != nil {
		return errors.Wrapf(err, "creating ip tables")
	}

	// for packets that leave the namespace and have the source IP address of the original guest, rewrite the source
	// address to clone address
	err = ipt.Delete("nat", "POSTROUTING", "-o", vethVmName, "-s", hostIp, "-j", "SNAT", "--to", cloneIp, "--wait")
	if err != nil {
		return errors.Wrapf(err, "deleting iptable POSTROUTING rule")
	}

	// do the reverse operation; rewrites the destination address of packets heading towards the clone
	// address to source address
	err = ipt.Delete("nat", "PREROUTING", "-i", vethVmName, "-d", cloneIp, "-j", "DNAT", "--to", hostIp, "--wait")
	if err != nil {
		return errors.Wrapf(err, "deleting iptable POSTROUTING rule")
	}

	return nil
}

func setupForwardRules(vethHostName, hostIface string) error {
	ipt, err := iptables.New()
	if err != nil {
		return errors.Wrapf(err, "creating ip tables")
	}

	err = ipt.Append("filter", "FORWARD", "-i", vethHostName, "-o", hostIface, "-j", "ACCEPT", "--wait")
	if err != nil {
		return errors.Wrapf(err, "adding iptable FORWARD rule")
	}

	err = ipt.Append("filter", "FORWARD", "-o", vethHostName, "-i", hostIface, "-j", "ACCEPT", "--wait")
	if err != nil {
		return errors.Wrapf(err, "adding iptable FORWARD rule")
	}
	return nil
}

func deleteForwardRules(vethHostName, hostIface string) error {
	ipt, err := iptables.New()
	if err != nil {
		return errors.Wrapf(err, "creating ip tables")
	}

	err = ipt.Delete("filter", "FORWARD", "-i", vethHostName, "-o", hostIface, "-j", "ACCEPT", "--wait")
	if err != nil {
		return errors.Wrapf(err, "deleting iptable FORWARD rule")
	}

	err = ipt.Delete("filter", "FORWARD", "-o", vethHostName, "-i", hostIface, "-j", "ACCEPT", "--wait")
	if err != nil {
		return errors.Wrapf(err, "deleting iptable FORWARD rule")
	}
	return nil
}

func addRoute(destIp, gatewayIp string) error {
	_, dstNet, err := net.ParseCIDR(fmt.Sprintf("%s/32", destIp))
	if err != nil {
		return errors.Wrapf(err, "parsing route destination ip")
	}

	gwAddr, _, err := net.ParseCIDR(gatewayIp)
	if err != nil {
		return errors.Wrapf(err, "parsing route gateway ip")
	}

	route := &netlink.Route{
		Dst: dstNet,
		Gw:  gwAddr,
	}

	if err := netlink.RouteAdd(route); err != nil {
		return errors.Wrapf(err, "adding route")
	}
	return nil
}

func deleteRoute(destIp, gatewayIp string) error {
	_, dstNet, err := net.ParseCIDR(fmt.Sprintf("%s/32", destIp))
	if err != nil {
		return errors.Wrapf(err, "parsing route destination ip")
	}

	gwAddr, _, err := net.ParseCIDR(gatewayIp)
	if err != nil {
		return errors.Wrapf(err, "parsing route gateway ip")
	}

	route := &netlink.Route{
		Dst: dstNet,
		Gw:  gwAddr,
	}

	if err := netlink.RouteDel(route); err != nil {
		return errors.Wrapf(err, "deleting route")
	}
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func getNetworkStartID() (int, error) {
	files, err := ioutil.ReadDir("/run/netns")
	if err != nil {
		return 0, errors.Wrapf(err, "Couldn't read network namespace dir")
	}

	maxId := 0
	for _, f := range files {
		if !f.IsDir() {
			netnsName := f.Name()

			re := regexp.MustCompile(`^uvmns([0-9]+)$`)
			regres := re.FindStringSubmatch(netnsName)

			if len(regres) > 1 {
				id, err := strconv.Atoi(regres[1])
				if err == nil {
					maxId = max(id, maxId)
				}
			}
		}
	}

	return maxId + 1, nil
}
