// MIT License
//
// Copyright (c) 2021 Amory Hoste and EASE lab
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
	"fmt"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netns"
	"net"
	"runtime"
)

const (
	defaultContainerCIDR = "172.16.0.2/24"
	defaultGatewayCIDR = "172.16.0.1/24"
	defaultContainerTap = "tap0"
	defaultContainerMac = "AA:FC:00:00:00:01"
)

// NetworkConfig represents the network devices, IPs, namespaces, routes and filter rules to connect a uVM
// to the network. The network config ID is deterministically mapped to IP addresses to be used for the uVM.
// Note that due to the current allocation of IPs at most 2^14 VMs can be simultaneously be available on a single host.
type NetworkConfig struct {
	id             int
	containerCIDR  string // Container IP address (CIDR notation)
	gatewayCIDR    string // Container gateway IP address (CIDR notation)
	containerTap   string // Container tap name
	containerMac   string // Container Mac address
	hostIfaceName  string // Host network interface name
}

// NewNetworkConfig creates a new network config with a given id and default host interface
func NewNetworkConfig(id int, hostIfaceName string) *NetworkConfig {
	return &NetworkConfig{
		id: id,
		containerCIDR: defaultContainerCIDR,
		gatewayCIDR: defaultGatewayCIDR,
		containerTap: defaultContainerTap,
		containerMac: defaultContainerMac,
		hostIfaceName: hostIfaceName,
	}
}

// GetMacAddress returns the mac address used for the uVM
func (cfg *NetworkConfig) GetMacAddress() string {
	return cfg.containerMac
}

// GetHostDevName returns the device connecting the uVM to the host
func (cfg *NetworkConfig) GetHostDevName() string {
	return cfg.containerTap
}

// getVeth0Name returns the name for the veth device at the side of the uVM
func (cfg *NetworkConfig) getVeth0Name() string {
	return fmt.Sprintf("veth%d-0", cfg.id)
}

// getVeth0CIDR returns the IP address for the veth device at the side of the uVM in CIDR notation
func (cfg *NetworkConfig) getVeth0CIDR() string {
	return fmt.Sprintf("172.17.%d.%d/30", (4 * cfg.id) / 256, ((4 * cfg.id) + 2) % 256)
}

// getVeth1Name returns the name for the veth device at the side of the host
func (cfg *NetworkConfig) getVeth1Name() string {
	return fmt.Sprintf("veth%d-1", cfg.id)
}

// getVeth1Name returns the IP address for the veth device at the side of the host in CIDR notation
func (cfg *NetworkConfig) getVeth1CIDR() string {
	return fmt.Sprintf("172.17.%d.%d/30", (4 * cfg.id) / 256, ((4 * cfg.id) + 1) % 256)
}

// GetCloneIP returns the IP address the uVM is reachable at from the host
func (cfg *NetworkConfig) GetCloneIP() string {
	return fmt.Sprintf("172.18.%d.%d", cfg.id / 254, 1 + (cfg.id % 254))
}

// GetContainerCIDR returns the internal IP of the uVM in CIDR notation
func (cfg *NetworkConfig) GetContainerCIDR() string {
	return cfg.containerCIDR
}

// getNamespaceName returns the network namespace name for the uVM
func (cfg *NetworkConfig) getNamespaceName() string {
	return fmt.Sprintf("uvmns%d", cfg.id)
}

// GetNamespacePath returns the full path to the network namespace for the uVM
func (cfg *NetworkConfig) GetNamespacePath() string {
	return fmt.Sprintf("/var/run/netns/%s", cfg.getNamespaceName())
}

// getContainerIP returns the internal IP of the uVM
func (cfg *NetworkConfig) getContainerIP() string {
	ip, _, _ := net.ParseCIDR(cfg.containerCIDR)
	return ip.String()
}

// GetGatewayIP returns the IP address of the tap device associated with the uVM
func (cfg *NetworkConfig) GetGatewayIP() string {
	ip, _, _ := net.ParseCIDR(cfg.gatewayCIDR)
	return ip.String()
}

// createVmNetwork creates network devices, namespaces, routes and filter rules for the uVM at the
// uVM side
func (cfg *NetworkConfig) createVmNetwork(hostNsHandle netns.NsHandle) error {
	// A. In uVM netns
	// A.1. Create network namespace for uVM & join network namespace
	vmNsHandle, err := netns.NewNamed(cfg.getNamespaceName()) // Switches namespace
	if err != nil {
		log.Println(err)
		return err
	}
	defer vmNsHandle.Close()

	// A.2. Create tap device for uVM
	if err := createTap(cfg.containerTap, cfg.gatewayCIDR, cfg.getNamespaceName()); err != nil {
		return err
	}

	// A.3. Create veth pair for uVM
	// A.3.1 Create veth pair
	if err := createVethPair(cfg.getVeth0Name(), cfg.getVeth1Name(), vmNsHandle, hostNsHandle); err != nil {
		return err
	}

	// A.3.2 Configure uVM side veth pair
	if err := configVeth(cfg.getVeth0Name(), cfg.getVeth0CIDR()); err != nil {
		return err
	}

	// A.3.3 Designate host side as default gateway for packets leaving namespace
	if err := setDefaultGateway(cfg.getVeth1CIDR()); err != nil {
		return err
	}

	// A.4. Setup NAT rules
	if err := setupNatRules(cfg.getVeth0Name(), cfg.getContainerIP(), cfg.GetCloneIP(), vmNsHandle); err != nil {
		return err
	}

	return nil
}

// createHostNetwork creates network devices, namespaces, routes and filter rules for the uVM at the host
// side
func (cfg *NetworkConfig) createHostNetwork() error {
	// B. In host netns
	// B.1 Configure host side veth pair
	if err := configVeth(cfg.getVeth1Name(), cfg.getVeth1CIDR()); err != nil {
		return err
	}

	// B.2 Add a route on the host for the clone address
	if err := addRoute(cfg.GetCloneIP(), cfg.getVeth0CIDR()); err != nil {
		return err
	}

	// B.3 Setup nat to route traffic out of veth device
	if err := setupForwardRules(cfg.getVeth1Name(), cfg.hostIfaceName); err != nil {
		return err
	}
	return nil
}

// CreateNetwork creates the necessary network devices, namespaces, routes and filter rules to connect the uVM to the
// network. The networking is created as described in the Firecracker documentation on providing networking for clones
// (https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/network-for-clones.md)
func (cfg *NetworkConfig) CreateNetwork() error {
	// 1. Lock the OS Thread so we don't accidentally switch namespaces
	runtime.LockOSThread()

	// 2. Get host network namespace
	hostNsHandle, err := netns.Get()
	defer hostNsHandle.Close()
	if err != nil {
		log.Printf("Failed to get host ns, %s\n", err)
		return err
	}

	// 3. Setup networking in instance namespace
	if err := cfg.createVmNetwork(hostNsHandle); err != nil {
		_ = netns.Set(hostNsHandle)
		runtime.UnlockOSThread()
		return err
	}

	// 4. Go back to host namespace
	err = netns.Set(hostNsHandle)
	if err != nil {
		return err
	}

	runtime.UnlockOSThread()

	// 5. Setup networking in host namespace
	if err := cfg.createHostNetwork(); err != nil {
		return err
	}

	return nil
}

// CreateNetwork removes the necessary network devices, namespaces, routes and filter rules to connect the
// function instance to the network
func (cfg *NetworkConfig) RemoveNetwork() error {
	// Delete nat to route traffic out of veth device
	if err := deleteForwardRules(cfg.getVeth1Name()); err != nil {
		return err
	}

	// Delete route on the host for the clone address
	if err := deleteRoute(cfg.GetCloneIP(), cfg.getVeth0CIDR()); err != nil {
		return err
	}

	runtime.LockOSThread()

	hostNsHandle, err := netns.Get()
	defer hostNsHandle.Close()
	if err != nil {
		log.Printf("Failed to get host ns, %s\n", err)
		return err
	}

	// Get uVM namespace handle
	vmNsHandle, err := netns.GetFromName(cfg.getNamespaceName())
	defer vmNsHandle.Close()
	if err != nil {
		return err
	}
	err = netns.Set(vmNsHandle)
	if err != nil {
		return err
	}

	// Delete NAT rules
	if err := deleteNatRules(vmNsHandle); err != nil {
		return err
	}

	// Delete default gateway for packets leaving namespace
	if err := deleteDefaultGateway(cfg.getVeth1CIDR()); err != nil {
		return err
	}

	// Delete uVM side veth pair
	if err := deleteVethPair(cfg.getVeth0Name(), cfg.getVeth1Name(), vmNsHandle, hostNsHandle); err != nil {
		return err
	}

	// Delete tap device for uVM
	if err := deleteTap(cfg.containerTap); err != nil {
		return err
	}

	// Delete namespace
	if err := netns.DeleteNamed(cfg.getNamespaceName()); err != nil {
		return errors.Wrapf(err, "deleting network namespace")
	}

	err = netns.Set(hostNsHandle)
	if err != nil {
		return err
	}
	runtime.UnlockOSThread()

	return nil
}