// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov and EASE lab
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

package misc

import (
	"fmt"
	"github.com/ease-lab/vhive/networking"
	"github.com/ease-lab/vhive/taps"
	"sync"

	"github.com/containerd/containerd"
)

const (
	defaultVcpuCount  = 1
	defaultMemsizeMib = 256
)

// VM type
type VM struct {
	ID               string
	ContainerSnapKey string
	SnapBooted       bool
	RemoteSnapBooted bool
	Image            *containerd.Image
	Container        *containerd.Container
	Task             *containerd.Task
	TaskCh           <-chan containerd.ExitStatus
	VCPUCount        uint32
	MemSizeMib       uint32
	netConfig        *networking.NetworkConfig
	ni               *taps.NetworkInterface
}

// GetIP returns the IP at which the VM is reachable
func (vm *VM) GetIP() string {
	if vm.netConfig != nil {
		return vm.netConfig.GetCloneIP()
	} else {
		return vm.ni.PrimaryAddress
	}
}

// GetMacAddress returns the name of the VM MAC address
func (vm *VM) GetMacAddress() string {
	if vm.netConfig != nil {
		return vm.netConfig.GetMacAddress()
	} else {
		return vm.ni.MacAddress
	}
}

// GetHostDevName returns the name of the VM host device
func (vm *VM) GetHostDevName() string {
	if vm.netConfig != nil {
		return vm.netConfig.GetHostDevName()
	} else {
		return vm.ni.HostDevName
	}
}

// GetPrimaryAddr returns the primary IP address of the VM
func (vm *VM) GetPrimaryAddr() string {
	if vm.netConfig != nil {
		return vm.netConfig.GetContainerCIDR()
	} else {
		return vm.ni.PrimaryAddress + vm.ni.Subnet
	}
}

func (vm *VM) GetGatewayAddr() string {
	if vm.netConfig != nil {
		return vm.netConfig.GetGatewayIP()
	} else {
		return vm.ni.GatewayAddress
	}
}

func (vm *VM) GetNetworkNamespace() string {
	if vm.netConfig != nil {
		return vm.netConfig.GetNamespacePath()
	} else {
		return ""
	}
}

// VMPool Pool of active VMs (can be in several states though)
type VMPool struct {
	vmMap       sync.Map
	isFullLocal bool
	// Used to create network for fullLocal snapshot VMs
	networkManager *networking.NetworkManager
	// Used to create snapshots for regular VMs
	tapManager *taps.TapManager
}

// NewVM Initialize a VM
func NewVM(vmID string) *VM {
	vm := new(VM)
	vm.ID = vmID
	vm.ContainerSnapKey = fmt.Sprintf("vm%s-containersnap", vmID)
	vm.SnapBooted = false
	vm.RemoteSnapBooted = false
	vm.MemSizeMib = defaultMemsizeMib
	vm.VCPUCount = defaultVcpuCount

	return vm
}
