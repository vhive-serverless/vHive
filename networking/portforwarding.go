package networking

import (
	"fmt"
	"net"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const (
	dnatTableName     = "nat"
	dnatChainName     = "vhive-dnat"
	dnatChainPriority = -100 // Standard DNAT priority, before routing decisions
)

// setupDNATChains creates the nftables DNAT chains (prerouting and output)
// Uses the shared "nat" table and creates custom "vhive-dnat-pre" and "vhive-dnat-out" chains
// Operations are idempotent - safe to call multiple times
func setupDNATChains() error {
	conn, err := nftables.New()
	if err != nil {
		return errors.Wrapf(err, "creating nftables connection")
	}

	// Create nat table (idempotent)
	natTable := &nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   dnatTableName,
	}
	conn.AddTable(natTable)

	// Create chains (idempotent)
	preroutingChainName := dnatChainName + "-pre"
	outputChainName := dnatChainName + "-out"
	polAccept := nftables.ChainPolicyAccept

	// Create prerouting chain for external traffic
	conn.AddChain(&nftables.Chain{
		Name:     preroutingChainName,
		Table:    natTable,
		Type:     nftables.ChainTypeNAT,
		Hooknum:  nftables.ChainHookPrerouting,
		Priority: nftables.ChainPriorityRef(dnatChainPriority),
		Policy:   &polAccept,
	})

	// Create output chain for local traffic
	conn.AddChain(&nftables.Chain{
		Name:     outputChainName,
		Table:    natTable,
		Type:     nftables.ChainTypeNAT,
		Hooknum:  nftables.ChainHookOutput,
		Priority: nftables.ChainPriorityRef(dnatChainPriority),
		Policy:   &polAccept,
	})

	if err := conn.Flush(); err != nil {
		return errors.Wrapf(err, "creating DNAT chains")
	}

	log.WithFields(log.Fields{
		"table":  dnatTableName,
		"chains": fmt.Sprintf("%s, %s", preroutingChainName, outputChainName),
	}).Debug("Setup DNAT chains")

	return nil
}

// addDNATRuleToChain adds a single DNAT rule to the specified chain
func addDNATRuleToChain(conn *nftables.Conn, table *nftables.Table, chain *nftables.Chain,
	sourceIP net.IP, sourcePort uint16, destIP net.IP, destPort uint16, networkID int) error {

	ruleExprs := []expr.Any{
		// Match protocol TCP
		&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     []byte{unix.IPPROTO_TCP},
		},
		// Match destination IP
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       16, // Destination IP offset in IP header
			Len:          4,  // IPv4 address length
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     sourceIP,
		},
		// Match destination port
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseTransportHeader,
			Offset:       2, // Destination port offset in TCP header
			Len:          2, // Port length
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     []byte{byte(sourcePort >> 8), byte(sourcePort)}, // Network byte order (big endian)
		},
		// DNAT to target IP and port
		&expr.Immediate{
			Register: 1,
			Data:     destIP,
		},
		&expr.Immediate{
			Register: 2,
			Data:     []byte{byte(destPort >> 8), byte(destPort)},
		},
		&expr.NAT{
			Type:        expr.NATTypeDestNAT,
			Family:      uint32(nftables.TableFamilyIPv4),
			RegAddrMin:  1,
			RegProtoMin: 2,
		},
	}

	rule := &nftables.Rule{
		Table:    table,
		Chain:    chain,
		Exprs:    ruleExprs,
		UserData: []byte(fmt.Sprintf("nexus-dnat-%d", networkID)),
	}

	conn.AddRule(rule)
	return nil
}

// addDNATRule adds DNAT rules to redirect traffic from sourceIP:sourcePort to destIP:destPort
// Rules are added to both prerouting (external traffic) and output (local traffic) chains
// networkID is used for UserData tagging to enable per-network rule deletion
func addDNATRule(sourceIP string, sourcePort int, destIP string, destPort int, networkID int) error {
	// Validate and parse IPs
	srcIP := net.ParseIP(sourceIP)
	if srcIP == nil {
		return errors.Errorf("invalid source IP: %s", sourceIP)
	}
	dstIP := net.ParseIP(destIP)
	if dstIP == nil {
		return errors.Errorf("invalid destination IP: %s", destIP)
	}

	// Convert to 4-byte representation
	srcIP = srcIP.To4()
	dstIP = dstIP.To4()
	if srcIP == nil || dstIP == nil {
		return errors.Errorf("IPs must be IPv4")
	}

	// Validate ports
	if sourcePort < 1 || sourcePort > 65535 || destPort < 1 || destPort > 65535 {
		return errors.Errorf("invalid port numbers: source=%d, dest=%d", sourcePort, destPort)
	}

	logger := log.WithFields(log.Fields{
		"networkID":  networkID,
		"sourceIP":   sourceIP,
		"sourcePort": sourcePort,
		"destIP":     destIP,
		"destPort":   destPort,
	})

	// Create connection
	conn, err := nftables.New()
	if err != nil {
		return errors.Wrapf(err, "creating nftables connection")
	}

	// Get table and chains
	natTable := &nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   dnatTableName,
	}

	preroutingChainName := dnatChainName + "-pre"
	outputChainName := dnatChainName + "-out"

	preroutingChain := &nftables.Chain{
		Name:  preroutingChainName,
		Table: natTable,
	}

	outputChain := &nftables.Chain{
		Name:  outputChainName,
		Table: natTable,
	}

	// Add rule to prerouting chain
	if err := addDNATRuleToChain(conn, natTable, preroutingChain, srcIP, uint16(sourcePort),
		dstIP, uint16(destPort), networkID); err != nil {
		return errors.Wrapf(err, "adding rule to prerouting chain")
	}

	// Add rule to output chain
	if err := addDNATRuleToChain(conn, natTable, outputChain, srcIP, uint16(sourcePort),
		dstIP, uint16(destPort), networkID); err != nil {
		return errors.Wrapf(err, "adding rule to output chain")
	}

	// Commit both rules
	if err := conn.Flush(); err != nil {
		return errors.Wrapf(err, "flushing DNAT rules")
	}

	logger.Debug("Added DNAT rules to prerouting and output chains")
	return nil
}

// deleteDNATRule removes DNAT rules for a specific network by matching UserData tag
func deleteDNATRule(networkID int) error {
	logger := log.WithFields(log.Fields{"networkID": networkID})

	conn, err := nftables.New()
	if err != nil {
		return errors.Wrapf(err, "creating nftables connection")
	}

	natTable := &nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   dnatTableName,
	}

	preroutingChainName := dnatChainName + "-pre"
	outputChainName := dnatChainName + "-out"

	// Get chains
	chains, err := conn.ListChainsOfTableFamily(nftables.TableFamilyIPv4)
	if err != nil {
		return errors.Wrapf(err, "listing chains")
	}

	var preroutingChain, outputChain *nftables.Chain
	for _, c := range chains {
		if c.Table.Name == dnatTableName {
			if c.Name == preroutingChainName {
				preroutingChain = c
			} else if c.Name == outputChainName {
				outputChain = c
			}
		}
	}

	if preroutingChain == nil || outputChain == nil {
		logger.Warn("DNAT chains not found, skipping rule deletion")
		return nil
	}

	userData := []byte(fmt.Sprintf("nexus-dnat-%d", networkID))
	rulesDeleted := 0

	// Delete from prerouting chain
	preroutingRules, err := conn.GetRules(natTable, preroutingChain)
	if err != nil {
		return errors.Wrapf(err, "getting prerouting rules")
	}

	for _, rule := range preroutingRules {
		if string(rule.UserData) == string(userData) {
			if err := conn.DelRule(rule); err != nil {
				return errors.Wrapf(err, "deleting prerouting rule")
			}
			rulesDeleted++
		}
	}

	// Delete from output chain
	outputRules, err := conn.GetRules(natTable, outputChain)
	if err != nil {
		return errors.Wrapf(err, "getting output rules")
	}

	for _, rule := range outputRules {
		if string(rule.UserData) == string(userData) {
			if err := conn.DelRule(rule); err != nil {
				return errors.Wrapf(err, "deleting output rule")
			}
			rulesDeleted++
		}
	}

	if err := conn.Flush(); err != nil {
		return errors.Wrapf(err, "flushing rule deletions")
	}

	logger.WithFields(log.Fields{"rulesDeleted": rulesDeleted}).Debug("Deleted DNAT rules")
	return nil
}

// flushDNATChains removes all rules from DNAT chains
func flushDNATChains() error {
	conn, err := nftables.New()
	if err != nil {
		return errors.Wrapf(err, "creating nftables connection")
	}

	natTable := &nftables.Table{
		Family: nftables.TableFamilyIPv4,
		Name:   dnatTableName,
	}

	preroutingChainName := dnatChainName + "-pre"
	outputChainName := dnatChainName + "-out"

	chains, err := conn.ListChainsOfTableFamily(nftables.TableFamilyIPv4)
	if err != nil {
		return errors.Wrapf(err, "listing chains")
	}

	rulesDeleted := 0

	for _, c := range chains {
		if c.Table.Name == dnatTableName && (c.Name == preroutingChainName || c.Name == outputChainName) {
			rules, err := conn.GetRules(natTable, c)
			if err != nil {
				return errors.Wrapf(err, "getting rules from chain %s", c.Name)
			}

			for _, rule := range rules {
				conn.DelRule(rule)
				rulesDeleted++
			}
		}
	}

	if err := conn.Flush(); err != nil {
		return errors.Wrapf(err, "flushing chain deletions")
	}

	log.WithFields(log.Fields{
		"rulesDeleted": rulesDeleted,
	}).Debug("Flushed DNAT chains")
	return nil
}

// deleteDNATChains removes the DNAT chains and table
func deleteDNATChains() error {
	conn, err := nftables.New()
	if err != nil {
		return errors.Wrapf(err, "creating nftables connection")
	}

	preroutingChainName := dnatChainName + "-pre"
	outputChainName := dnatChainName + "-out"

	chains, err := conn.ListChainsOfTableFamily(nftables.TableFamilyIPv4)
	if err != nil {
		return errors.Wrapf(err, "listing chains")
	}

	for _, c := range chains {
		if c.Table.Name == dnatTableName && (c.Name == preroutingChainName || c.Name == outputChainName) {
			conn.DelChain(c)
		}
	}

	if err := conn.Flush(); err != nil {
		return errors.Wrapf(err, "deleting DNAT chains")
	}

	log.WithFields(log.Fields{
		"chains": fmt.Sprintf("%s, %s", preroutingChainName, outputChainName),
	}).Debug("Deleted DNAT chains")
	return nil
}
