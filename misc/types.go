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
    "fmt"
    "sync"

    "github.com/containerd/containerd"
    "google.golang.org/grpc"

    hpb "github.com/ustiugov/fccd-orchestrator/helloworld"
)

type NetworkInterface struct {
    MacAddress string
    HostDevName string
    PrimaryAddress string
    Subnet string
    GatewayAddress string
}

type VM struct {
    vmID string
    functionID string // unused

    Image *containerd.Image
    Container *containerd.Container
    Task *containerd.Task
    TaskCh <-chan containerd.ExitStatus
    Ni *NetworkInterface
    Conn *grpc.ClientConn
    FuncClient *hpb.GreeterClient

    isOffloaded bool // reserved
    isPrewarming bool // reserved
    isActive bool

    // Transient states
    isStarting bool
    isDeactivating bool
}

type NiPool struct {
    mu *sync.Mutex
    niList []NetworkInterface
}

type VmPool struct {
    mu *sync.Mutex
    vmMap map[string]VM
}


func NewVM(vmID string) (*VM) {
    vm := new(VM)
    vm.vmID = vmID

    return vm
}

func (vm *VM) Sprintf() string {
    return fmt.Sprintf("%s/%s: state:S=%t|A=%t|D=%t", vm.vmID, vm.vmID, vm.isStarting, // TODO: vmID ->fID
                       vm.isActive, vm.isDeactivating)
}
/*
State-machine transitioning functions:

 x -> Starting or Prewarming (tranient) -> Active -> 
 -> Deactivating or Offloading (transient) -> x or Offloaded

 Note: concurrent mix of prewarming and starting is not supported

 */

func (vm *VM) SetStateStarting() {
    if vm.isActive == true || vm.isDeactivating == true || vm.isStarting == true {
        panic("SetStateStarting")
    }

    vm.isStarting = true
}

//TODO: setStateOflloading

func (vm *VM) SetStateActive() {
    if vm.isActive == true || vm.isDeactivating == true || vm.isStarting == false {
        panic("SetStateActive")
    }

    vm.isStarting = false
    vm.isActive = true
}

func (vm *VM) SetStateDeactivating() {
    if vm.isActive == false || vm.isDeactivating == true || vm.isStarting == true {
        panic("SetStateDeactivating")
    }

    vm.isActive = false
    vm.isDeactivating = true
}

