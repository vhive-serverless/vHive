// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Dmitrii Ustiugov and vHive team
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
	"github.com/google/uuid"
	"sync"

	"github.com/containerd/containerd"

	"github.com/vhive-serverless/vhive/networking"
)

// VM type
type VM struct {
	ID               string
	ContainerSnapKey string
	SnapBooted       bool
	Image            *containerd.Image
	Container        *containerd.Container
	Task             *containerd.Task
	TaskCh           <-chan containerd.ExitStatus
	NetConfig        *networking.NetworkConfig
}

// VMPool Pool of active VMs (can be in several states though)
type VMPool struct {
	vmMap          sync.Map
	networkManager *networking.NetworkManager
}

// NewVM Initialize a VM
func NewVM(vmID string) *VM {
	vm := new(VM)
	vm.ID = vmID
	vm.ContainerSnapKey = fmt.Sprintf("vm%s-containersnap-%s", vmID, (uuid.New()).String()[:16])
	vm.SnapBooted = false

	return vm
}

// GetIP returns the IP at which the VM is reachable
func (vm *VM) GetIP() string {
	return vm.NetConfig.GetCloneIP()
}

// GetMacAddress returns the name of the VM MAC address
func (vm *VM) GetMacAddress() string {
	return vm.NetConfig.GetMacAddress()
}

// GetHostDevName returns the name of the VM host device
func (vm *VM) GetHostDevName() string {
	return vm.NetConfig.GetHostDevName()
}

// GetPrimaryAddr returns the primary IP address of the VM
func (vm *VM) GetPrimaryAddr() string {
	return vm.NetConfig.GetContainerCIDR()
}

func (vm *VM) GetGatewayAddr() string {
	return vm.NetConfig.GetGatewayIP()
}

func (vm *VM) GetNetworkNamespace() string {
	return vm.NetConfig.GetNamespacePath()
}
