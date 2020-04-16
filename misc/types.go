// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov
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
	"sync"

	"github.com/containerd/containerd"
	"google.golang.org/grpc"

	hpb "github.com/ustiugov/fccd-orchestrator/helloworld"
)

// NetworkInterface Network interface type, NI names are generated based on expected tap names
type NetworkInterface struct {
	MacAddress     string
	HostDevName    string
	PrimaryAddress string
	Subnet         string
	GatewayAddress string
}

// VM type
type VM struct {
	vmID string
	//	functionID string // unused

	Image      *containerd.Image
	Container  *containerd.Container
	Task       *containerd.Task
	TaskCh     <-chan containerd.ExitStatus
	Ni         *NetworkInterface
	Conn       *grpc.ClientConn
	FuncClient *hpb.GreeterClient

	//	isOffloaded  bool // reserved
	//	isPrewarming bool // reserved
	isActive bool

	// Transient states
	isStarting     bool
	isDeactivating bool
}

// NiPool Pool of NIs
type NiPool struct {
	mu     *sync.Mutex
	niList []NetworkInterface
}

// VMPool Pool of active VMs (can be in several states though)
type VMPool struct {
	mu    *sync.Mutex
	vmMap map[string]*VM
}

// NewVM Initialize a VM
func NewVM(vmID string) *VM {
	vm := new(VM)
	vm.vmID = vmID

	return vm
}

/*
State-machine transitioning functions:

 x -> Starting or Prewarming (tranient) -> Active ->
 -> Deactivating or Offloading (transient) -> x or Offloaded

 Note: concurrent mix of prewarming and starting is not supported

*/

// SetStateStarting From x to Starting
func (vm *VM) setStateStarting() {
	if vm.isActive || vm.isDeactivating || vm.isStarting {
		panic("SetStateStarting")
	}

	vm.isStarting = true
}

//TODO: setStateOflloading

// SetStateActive From Starting to Active
func (vm *VM) setStateActive() {
	if vm.isActive || vm.isDeactivating || !vm.isStarting {
		panic("SetStateActive")
	}

	vm.isStarting = false
	vm.isActive = true
}

// SetStateDeactivating From Active to Deactivating
func (vm *VM) setStateDeactivating() {
	if !vm.isActive || vm.isDeactivating || vm.isStarting {
		panic("SetStateDeactivating")
	}

	vm.isActive = false
	vm.isDeactivating = true
}
