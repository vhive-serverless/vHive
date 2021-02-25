// MIT License
//
// Copyright (c) 2020 Plamen Petrov and EASE lab
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

package taps

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync/atomic"

	log "github.com/sirupsen/logrus"

	"net"

	"github.com/vishvananda/netlink"
)

// getGatewayAddr Creates the gateway address (first address in pool)
func getGatewayAddr(bridgeID int) string {
	return fmt.Sprintf("19%d.128.0.1", bridgeID)
}

// getBridgeName Create bridge name
func getBridgeName(id int) string {
	return fmt.Sprintf("br%d", id)
}

// getPrimaryAddress Creates the primary address for a tap
func getPrimaryAddress(curTaps, bridgeID int) string {
	return fmt.Sprintf("19%d.128.%d.%d", bridgeID, (curTaps+2)/256, (curTaps+2)%256)
}

// NewTapManager Creates a new tap manager
func NewTapManager() *TapManager {
	tm := new(TapManager)

	tm.numBridges = NumBridges
	tm.TapCountsPerBridge = make([]int64, NumBridges)
	tm.createdTaps = make(map[string]*NetworkInterface)

	log.Info("Registering bridges for tap manager")

	for i := 0; i < NumBridges; i++ {
		brName := getBridgeName(i)
		gatewayAddr := getGatewayAddr(i)

		createBridge(brName, gatewayAddr)
	}

	return tm
}

// Creates the bridge, add a gateway to it, and enables it
func createBridge(bridgeName, gatewayAddr string) {
	logger := log.WithFields(log.Fields{"bridge": bridgeName})

	logger.Debug("Creating bridge")

	la := netlink.NewLinkAttrs()
	la.Name = bridgeName

	br := &netlink.Bridge{LinkAttrs: la}

	if err := netlink.LinkAdd(br); err != nil {
		logger.Panic("Bridge could not be created")
	}

	if err := netlink.LinkSetUp(br); err != nil {
		logger.Panic("Bridge could not be enabled")
	}

	bridgeAddress := gatewayAddr + Subnet

	addr, err := netlink.ParseAddr(bridgeAddress)
	if err != nil {
		log.Panic(fmt.Sprintf("could not parse bridge address %s", bridgeAddress))
	}

	if err := netlink.AddrAdd(br, addr); err != nil {
		logger.Panic(fmt.Sprintf("could not add %s to bridge", bridgeAddress))
	}
}

//ConfigIPtables Configures IP tables for internet access inside VM
func ConfigIPtables(tapName string) error {

	var hostIface = ""

	out, err := exec.Command(
		"route",
	).Output()
	if err != nil {
		log.Warnf("Failed to fetch host net interfaces %v\n%s\n", err, out)
		return err
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "default") {
			hostIface = line[strings.LastIndex(line, " ")+1:]
		}
	}

	cmd := exec.Command(
		"sudo", "iptables", "-t", "nat", "-A", "POSTROUTING", "-o", hostIface, "-j", "MASQUERADE",
	)
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to configure NAT %v\n%s\n", err, stdoutStderr)
		return err
	}
	cmd = exec.Command(
		"sudo", "iptables", "-A", "FORWARD", "-i", tapName, "-o", hostIface, "-j", "ACCEPT",
	)
	stdoutStderr, err = cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to setup forwarding into tap %v\n%s\n", err, stdoutStderr)
		return err
	}
	cmd = exec.Command(
		"sudo", "iptables", "-A", "FORWARD", "-o", tapName, "-i", hostIface, "-j", "ACCEPT",
	)
	stdoutStderr, err = cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to setup forwarding out from tap %v\n%s\n", err, stdoutStderr)
		return err
	}
	cmd = exec.Command(
		"sudo", "iptables", "-A", "FORWARD", "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT",
	)
	stdoutStderr, err = cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to configure conntrack %v\n%s\n", err, stdoutStderr)
		return err
	}
	return nil
}

// AddTap Creates a new tap and returns the corresponding network interface
func (tm *TapManager) AddTap(tapName string) (*NetworkInterface, error) {
	tm.Lock()

	if ni, ok := tm.createdTaps[tapName]; ok {
		tm.Unlock()
		return ni, tm.reconnectTap(tapName, ni)
	}

	tm.Unlock()

	for i := 0; i < tm.numBridges; i++ {
		tapsInBridge := atomic.AddInt64(&tm.TapCountsPerBridge[i], 1)
		if tapsInBridge-1 < TapsPerBridge {
			// Create a tap with this bridge
			ni, err := tm.addTap(tapName, i, int(tapsInBridge-1))
			if err == nil {
				tm.Lock()
				tm.createdTaps[tapName] = ni
				tm.Unlock()
				err := ConfigIPtables(tapName)
				if err != nil {
					return nil, err
				}
			}

			return ni, err
		}
	}
	log.Error("No space for creating taps")
	return nil, errors.New("No space for creating taps")
}

// Reconnects a single tap with the same network interface that it was
// create with previously
func (tm *TapManager) reconnectTap(tapName string, ni *NetworkInterface) error {
	logger := log.WithFields(log.Fields{"tap": tapName, "bridge": ni.BridgeName})

	la := netlink.NewLinkAttrs()
	la.Name = tapName

	logger.Debug("Reconnecting tap")

	tap := &netlink.Tuntap{LinkAttrs: la, Mode: netlink.TUNTAP_MODE_TAP}

	if err := netlink.LinkAdd(tap); err != nil {
		logger.Error("Tap could not be reconnected")
		return err
	}

	br, err := netlink.LinkByName(ni.BridgeName)
	if err != nil {
		logger.Error("Could not reconnect tap, because corresponding bridge does not exist")
		return err
	}

	hwAddr, err := net.ParseMAC(ni.MacAddress)
	if err != nil {
		logger.Error("Could not parse MAC")
		return err
	}

	if err := netlink.LinkSetHardwareAddr(tap, hwAddr); err != nil {
		logger.Error("Could not set MAC address")
		return err
	}

	if err := netlink.LinkSetMaster(tap, br); err != nil {
		logger.Error("Master could not be set")
		return err
	}

	if err := netlink.LinkSetUp(tap); err != nil {
		logger.Error("Tap could not be enabled")
		return err
	}

	return nil
}

// Creates a single tap and connects it to the corresponding bridge
func (tm *TapManager) addTap(tapName string, bridgeID, currentNumTaps int) (*NetworkInterface, error) {
	bridgeName := getBridgeName(bridgeID)

	logger := log.WithFields(log.Fields{"tap": tapName, "bridge": bridgeName})

	la := netlink.NewLinkAttrs()
	la.Name = tapName

	logger.Debug("Creating tap")

	tap := &netlink.Tuntap{LinkAttrs: la, Mode: netlink.TUNTAP_MODE_TAP}

	if err := netlink.LinkAdd(tap); err != nil {
		logger.Error("Tap could not be created")
		return nil, err
	}

	br, err := netlink.LinkByName(bridgeName)
	if err != nil {
		logger.Error("Could not create tap, because corresponding bridge does not exist")
		return nil, err
	}

	if err := netlink.LinkSetMaster(tap, br); err != nil {
		logger.Error("Master could not be set")
		return nil, err
	}

	macIndex := bridgeID*TapsPerBridge + currentNumTaps
	macAddress := fmt.Sprintf("02:FC:00:00:%02X:%02X", macIndex/256, macIndex%256)

	hwAddr, err := net.ParseMAC(macAddress)
	if err != nil {
		logger.Error("Could not parse MAC")
		return nil, err
	}

	if err := netlink.LinkSetHardwareAddr(tap, hwAddr); err != nil {
		logger.Error("Could not set MAC address")
		return nil, err
	}

	if err := netlink.LinkSetUp(tap); err != nil {
		logger.Error("Tap could not be enabled")
		return nil, err
	}

	return &NetworkInterface{
		BridgeName:     bridgeName,
		MacAddress:     macAddress,
		PrimaryAddress: getPrimaryAddress(currentNumTaps, bridgeID),
		HostDevName:    tapName,
		Subnet:         Subnet,
		GatewayAddress: getGatewayAddr(bridgeID),
	}, nil
}

// RemoveTap Removes the tap
func (tm *TapManager) RemoveTap(tapName string) error {
	logger := log.WithFields(log.Fields{"tap": tapName})

	logger.Debug("Removing tap")

	tap, err := netlink.LinkByName(tapName)
	if err != nil {
		logger.Warn("Could not find tap")
		return nil
	}

	if err := netlink.LinkDel(tap); err != nil {
		logger.Error("Tap could not be removed")
		return err
	}

	return nil
}

// RemoveBridges Removes the bridges created by the tap manager
func (tm *TapManager) RemoveBridges() {
	log.Info("Removing bridges")
	for i := 0; i < tm.numBridges; i++ {
		bridgeName := getBridgeName(i)

		logger := log.WithFields(log.Fields{"bridge": bridgeName})

		br, err := netlink.LinkByName(bridgeName)
		if err != nil {
			logger.Warn("Could not find bridge")
			continue
		}

		if err := netlink.LinkDel(br); err != nil {
			logger.WithFields(log.Fields{"bridge": bridgeName}).Panic("Bridge could not be deleted")
		}
	}
}
