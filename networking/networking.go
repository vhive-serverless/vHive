// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Amory Hoste and vHive team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package networking

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

// getHostIfaceName returns the default host network interface name.
func getHostIfaceName() (string, error) {
	out, err := exec.Command(
		"route",
	).Output()
	if err != nil {
		log.Warnf("Failed to fetch host net interfaces %v\n%s\n", err, out)
		return "", err
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "default") {
			return line[strings.LastIndex(line, " ")+1:], nil
		}
	}
	return "", errors.New("Failed to fetch host net interface")
}

// getExperimentIfaceName returns the experiment host network interface name.
func getExperimentIfaceName() (string, error) {
	out, err := exec.Command(
		"route",
	).Output()
	if err != nil {
		log.Warnf("Failed to fetch host net interfaces %v\n%s\n", err, out)
		return "", err
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "10.0.1.0") {
			return line[strings.LastIndex(line, " ")+1:], nil
		}
	}
	return "", errors.New("Failed to fetch experiment net interface")
}

// createTap creates a TAP device with name tapName, IP gatewayIP in the network namespace with name netnsName
func createTap(tapName, gatewayIP, netnsName string) error {
	// 1. Create tap device

	logger := log.WithFields(log.Fields{"tap": tapName, "IP gateway": gatewayIP, "namespace": netnsName})

	la := netlink.NewLinkAttrs()
	la.Name = tapName
	la.Namespace = netnsName

	logger.Debug("Creating tap for virtual network")

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

// deleteTap deletes the tap device identified by name tapName
func deleteTap(tapName string) error {
	logger := log.WithFields(log.Fields{"tap": tapName})
	logger.Debug("Removing tap")
	if err := netlink.LinkDel(&netlink.Tuntap{LinkAttrs: netlink.LinkAttrs{Name: tapName}}); err != nil {
		return errors.Wrapf(err, "deleting tap %s", tapName)
	}

	return nil
}

// createVethPair creates a virtual ethernet pair connecting the supplied namespaces
func createVethPair(veth0Name, veth1Name string, veth0NsHandle, veth1NsHandle netns.NsHandle) error {
	veth := &netlink.Veth{LinkAttrs: netlink.LinkAttrs{Name: veth0Name, Namespace: netlink.NsFd(veth0NsHandle), TxQLen: 1000}, PeerName: veth1Name, PeerNamespace: netlink.NsFd(veth1NsHandle)}
	if err := netlink.LinkAdd(veth); err != nil {
		return errors.Wrapf(err, "creating veth pair")
	}

	return nil
}

// deleteVethPair deletes the virtual ethernet pair connecting the supplied namespaces
func deleteVethPair(veth0Name, veth1Name string, veth0NsHandle, veth1NsHandle netns.NsHandle) error {
	if err := netlink.LinkDel(&netlink.Veth{LinkAttrs: netlink.LinkAttrs{Name: veth0Name, Namespace: netlink.NsFd(veth0NsHandle)}, PeerName: veth1Name, PeerNamespace: netlink.NsFd(veth1NsHandle)}); err != nil {
		return errors.Wrapf(err, "deleting veth %s", veth0Name)
	}
	return nil
}

// configVeth configures the IP address of a veth device and enables the device
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

// setDefaultGateway creates a default routing rule to the supplied gatewayIP
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

// deleteDefaultGateway deletes the default routing rule to the supplied gatewayIP
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

// setupNatRules configures the NAT rules. Each uVMs address is translated to an external clone address to avoid
// conflicts (see https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/network-for-clones.md)
func setupNatRules(vethVmName, hostIp, cloneIp string, vmNsHandle netns.NsHandle) error {
	conn := nftables.Conn{NetNS: int(vmNsHandle)}

	// 1. add table ip nat
	natTable := &nftables.Table{
		Name:   "nat",
		Family: nftables.TableFamilyIPv4,
	}

	// 2. Iptables: -t nat -A POSTROUTING -o veth1-0 -s 172.16.0.2 -j SNAT --to 192.168.0.1
	// 2.1 add chain ip nat POSTROUTING { type nat hook postrouting priority 0; policy accept; }
	polAccept := nftables.ChainPolicyAccept
	postRouteCh := &nftables.Chain{
		Name:     "POSTROUTING",
		Table:    natTable,
		Type:     nftables.ChainTypeNAT,
		Priority: nftables.ChainPriorityRef(0),
		Hooknum:  nftables.ChainHookPostrouting,
		Policy:   &polAccept,
	}

	// 2.2 add rule ip nat POSTROUTING oifname veth1-0 ip saddr 172.16.0.2 counter snat to 192.168.0.1
	snatRule := &nftables.Rule{
		Table: natTable,
		Chain: postRouteCh,
		Exprs: []expr.Any{
			// Load iffname in register 1
			&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
			// Check iifname == veth1-0
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     []byte(fmt.Sprintf("%s\x00", vethVmName)),
			},
			// Load source IP address (offset 12 bytes network header) in register 1
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       12,
				Len:          4,
			},
			// Check source ip address == 172.16.0.2
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     net.ParseIP(hostIp).To4(),
			},
			// Load snatted address (192.168.0.1) in register 1
			&expr.Immediate{
				Register: 1,
				Data:     net.ParseIP(cloneIp).To4(),
			},
			&expr.NAT{
				Type:       expr.NATTypeSourceNAT, // Snat
				Family:     unix.NFPROTO_IPV4,
				RegAddrMin: 1,
			},
		},
	}

	// 3. Iptables: -t nat -A PREROUTING -i veth1-0 -d 192.168.0.1 -j DNAT --to 172.16.0.2
	// 3.1 add chain ip nat PREROUTING { type nat hook prerouting priority 0; policy accept; }
	preRouteCh := &nftables.Chain{
		Name:     "PREROUTING",
		Table:    natTable,
		Type:     nftables.ChainTypeNAT,
		Priority: nftables.ChainPriorityRef(0),
		Hooknum:  nftables.ChainHookPrerouting,
		Policy:   &polAccept,
	}

	// 3.2 add rule ip nat PREROUTING iifname veth1-0 ip daddr 192.168.0.1 counter dnat to 172.16.0.2
	dnatRule := &nftables.Rule{
		Table: natTable,
		Chain: preRouteCh,
		Exprs: []expr.Any{
			// Load iffname in register 1
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			// Check iifname == veth1-0
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     []byte(fmt.Sprintf("%s\x00", vethVmName)),
			},
			// Load destination IP address (offset 16 bytes network header) in register 1
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       16,
				Len:          4,
			},
			// Check destination ip address == 192.168.0.1
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     net.ParseIP(cloneIp).To4(),
			},
			// Load dnatted address (172.16.0.2) in register 1
			&expr.Immediate{
				Register: 1,
				Data:     net.ParseIP(hostIp).To4(),
			},
			&expr.NAT{
				Type:       expr.NATTypeDestNAT, // Dnat
				Family:     unix.NFPROTO_IPV4,
				RegAddrMin: 1,
			},
		},
	}

	// Apply rules
	conn.AddTable(natTable)
	conn.AddChain(postRouteCh)
	conn.AddRule(snatRule)
	conn.AddChain(preRouteCh)
	conn.AddRule(dnatRule)
	if err := conn.Flush(); err != nil {
		return errors.Wrapf(err, "creating nat rules")
	}
	return nil
}

// deleteNatRules deletes the NAT rules to give each uVM a clone address.
func deleteNatRules(vmNsHandle netns.NsHandle) error {
	conn := nftables.Conn{NetNS: int(vmNsHandle)}

	natTable := &nftables.Table{
		Name:   "nat",
		Family: nftables.TableFamilyIPv4,
	}

	// Apply
	conn.DelTable(natTable)
	if err := conn.Flush(); err != nil {
		return errors.Wrapf(err, "deleting nat rules")
	}
	return nil
}

// setupForwardRules creates forwarding rules to allow traffic from the end of the veth pair to the default host interface.
func setupForwardRules(vethHostName, hostIface string) error {
	conn := nftables.Conn{}

	// 1. add table ip filter
	filterTable := &nftables.Table{
		Name:   "filter",
		Family: nftables.TableFamilyIPv4,
	}

	// 2. add chain ip filter FORWARD { type filter hook forward priority 0; policy accept; }
	polAccept := nftables.ChainPolicyAccept
	fwdCh := &nftables.Chain{
		Name:     "vhive-forward",
		Table:    filterTable,
		Type:     nftables.ChainTypeFilter,
		Priority: nftables.ChainPriorityRef(0),
		Hooknum:  nftables.ChainHookForward,
		Policy:   &polAccept,
	}

	// 3. Iptables: -A FORWARD -i veth1-1 -o eno49 -j ACCEPT
	// 3.1 add rule ip filter FORWARD iifname veth1-1 oifname eno49 counter accept
	outRule := &nftables.Rule{
		Table:    filterTable,
		Chain:    fwdCh,
		UserData: []byte(fmt.Sprintf("vhive-forward-%s", vethHostName)),
		Exprs: []expr.Any{
			// Load iffname in register 1
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			// Check iifname == veth1-0
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     []byte(fmt.Sprintf("%s\x00", vethHostName)),
			},
			// Load oif in register 1
			&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
			// Check iifname == veth1-0
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     []byte(fmt.Sprintf("%s\x00", hostIface)),
			},
			&expr.Verdict{
				Kind: expr.VerdictAccept,
			},
		},
	}

	// 4. Iptables: -A FORWARD -o veth1-1 -i eno49 -j ACCEPT
	// 4.1 add rule ip filter FORWARD iifname eno49 oifname veth1-1 counter accept
	inRule := &nftables.Rule{
		Table:    filterTable,
		Chain:    fwdCh,
		UserData: []byte(fmt.Sprintf("vhive-forward-%s", vethHostName)),
		Exprs: []expr.Any{
			// Load oifname in register 1
			&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
			// Check iifname == veth1-0
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     []byte(fmt.Sprintf("%s\x00", vethHostName)),
			},
			// Load oif in register 1
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			// Check iifname == veth1-0
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     []byte(fmt.Sprintf("%s\x00", hostIface)),
			},
			&expr.Verdict{
				Kind: expr.VerdictAccept,
			},
		},
	}
	conn.AddTable(filterTable)
	conn.AddChain(fwdCh)
	conn.AddRule(outRule)
	conn.AddRule(inRule)

	if err := conn.Flush(); err != nil {
		return errors.Wrapf(err, "creating forward rules")
	}
	return nil
}

// deleteNatRules deletes the forward rules to allow traffic to the default host interface.
func deleteForwardRules(vethHostName string) error {
	conn := nftables.Conn{}

	// 1. add table ip filter
	filterTable := &nftables.Table{
		Name:   "filter",
		Family: nftables.TableFamilyIPv4,
	}

	// 2. Get shared forward chain
	fwdCh := &nftables.Chain{
		Name:  "vhive-forward",
		Table: filterTable,
	}

	// 3. Get all rules in the forward chain
	rules, err := conn.GetRules(filterTable, fwdCh)
	if err != nil {
		return errors.Wrapf(err, "deleting forward rules")
	}

	// 4. Delete all rules belonging to the veth interface
	for _, rule := range rules {
		if string(rule.UserData) == fmt.Sprintf("vhive-forward-%s", vethHostName) {
			if err := conn.DelRule(rule); err != nil {
				return errors.Wrapf(err, "deleting forward rules")
			}
		}
	}

	if err := conn.Flush(); err != nil {
		return errors.Wrapf(err, "deleting forward rules")
	}

	return nil
}

// setupMasquerade creates NAT rules for external communication from the uVM.
func setupMasquerade(vethHostName, hostIface string) error {
	conn := nftables.Conn{}

	// 1. add table ip nat
	natTable := &nftables.Table{
		Name:   "nat",
		Family: nftables.TableFamilyIPv4,
	}

	// 2. add chain ip nat POSTROUTING { type nat hook postrouting priority 0; policy accept; }
	polAccept := nftables.ChainPolicyAccept
	natCh := &nftables.Chain{
		Name:     "vhive-masq",
		Table:    natTable,
		Type:     nftables.ChainTypeNAT,
		Priority: nftables.ChainPriorityRef(0),
		Hooknum:  nftables.ChainHookPostrouting,
		Policy:   &polAccept,
	}

	// 3. add rule ip nat POSTROUTING iifname veth1-0 oifname eno1 counter masquerade
	masqRule := &nftables.Rule{
		Table:    natTable,
		Chain:    natCh,
		UserData: []byte(fmt.Sprintf("vhive-masq-%s", vethHostName)),
		Exprs: []expr.Any{
			// Load iffname in register 1
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			// Check iifname == veth1-0
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     []byte(fmt.Sprintf("%s\x00", vethHostName)),
			},
			// Load offname in register 2
			&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 2},
			// Check oifname == eno1
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 2,
				Data:     []byte(fmt.Sprintf("%s\x00", hostIface)),
			},
			// masq
			&expr.Masq{},
		},
	}

	// Apply
	conn.AddTable(natTable)
	conn.AddChain(natCh)
	conn.AddRule(masqRule)
	if err := conn.Flush(); err != nil {
		return errors.Wrapf(err, "creating masquerade rules")
	}
	return nil
}

// deleteMasquerade deletes the NAT rules for external communication from the uVM.
func deleteMasquerade(vethHostName string) error {
	conn := nftables.Conn{}

	// 1. add table ip nat
	natTable := &nftables.Table{
		Name:   "nat",
		Family: nftables.TableFamilyIPv4,
	}

	// 2. Get shared forward chain
	natCh := &nftables.Chain{
		Name:  "vhive-masq",
		Table: natTable,
	}

	// 3. Get all rules in the forward chain
	rules, err := conn.GetRules(natTable, natCh)
	if err != nil {
		return errors.Wrapf(err, "deleting masquerade rules")
	}

	// 4. Delete all rules belonging to the veth interface
	for _, rule := range rules {
		if string(rule.UserData) == fmt.Sprintf("vhive-masq-%s", vethHostName) {
			if err := conn.DelRule(rule); err != nil {
				return errors.Wrapf(err, "deleting masquerade rules")
			}
		}
	}

	if err := conn.Flush(); err != nil {
		return errors.Wrapf(err, "deleting masquerade rules")
	}

	return nil
}

// addRoute adds a routing table entry to destIp with gateway gatewayIp.
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

// addRoute deletes the routing table entry to destIp with gateway gatewayIp.
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

// getNetworkStartID fetches the
func getNetworkStartID() (int, error) {
	entries, err := os.ReadDir("/run/netns")
	if err != nil {
		return 0, errors.Wrapf(err, "Couldn't read network namespace dir")
	}

	maxId := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			netnsName := entry.Name()

			re := regexp.MustCompile(`^uvmns([0-9]+)$`)
			regres := re.FindStringSubmatch(netnsName)

			if len(regres) > 1 {
				id, err := strconv.Atoi(regres[1])
				if err == nil && id > maxId {
					maxId = id
				}
			}
		}
	}

	return maxId + 1, nil
}
