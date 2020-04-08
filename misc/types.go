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

func NewVM(vmID string) (*VM) {
    vm := new(misc.VM)
    vm.vmID = vmID

    return vm
}

/*
State-machine transitioing functions:

 x -> Starting or Prewarming (tranient) -> Active -> 
 -> Deactivating or Offloading (transient) -> x or Offloaded

 Note: concurrent mix of prewarming and starting is not supported

 */

func (vm *VM) SetStateStarting() {
    if isActive == true || vm.Deactivating == true || vm.Starting == true {
        panic("")
    }

    vm.Starting = true
}

func (vm *VM) SetStateDeactivating() {
    if isActive == false || vm.Deactivating == true || vm.Starting == true {
        panic("")
    }

    vm.isActive == false
    vm.Deactivating = true
}

//TODO: setStateOflloading

func (vm *VM) SetStateActive() {
    if isActive == true || vm.Deactivating == true || vm.Starting == false {
        panic("")
    }

    if vm.Starting == true {
        vm.Starting = false
    }

    vm.isActive = true
}

func (vm *VM) SetStateDeactivating() {
    if isActive == true || vm.Deactivating == true || vm.Starting == true {
        panic("")
    }

    vm.Active = false
    vm.Deactivating = true
}

// TODO: move VM and ni allocation here
type VmPool struct {
    mu *sync.Mutex
    vmMap map[string]VM
}

func NewVmPool() (*VmPool) {
    p := new(VmPool)
    p.mu = &sync.Mutex{}
    p.vmMap = make(map[string]VM)

    return p
}

type NiPool struct {
    mu *sync.Mutex
    niList []NetworkInterface
}

func NewNiPool(niNum int) (*NiPool) {
    p := new(NiPool)
    p.mu = &sync.Mutex{}

    for i := 0; i < niNum; i++ {
        ni := NetworkInterface{
            MacAddress: fmt.Sprintf("02:FC:00:00:%02X:%02X", i/256, i%256),
            HostDevName: fmt.Sprintf("fc-%d-tap0", i),
            PrimaryAddress: fmt.Sprintf("19%d.128.%d.%d", i%2+6, (i+2)/256, (i+2)%256),
            Subnet: "/10",
            GatewayAddress: fmt.Sprintf("19%d.128.0.1", i%2+6),
        }
        p.niList = append(p.niList, ni)
    }

    return p
}

/*
 Error types
 */
type NonExistErr string

func (e NonExistErr) Error() string {
    return fmt.Sprintf("%s does not exist", e)
}

type AlreadyStartingErr string

func (e AlreadyStartingErr) Error() string {
    return fmt.Sprintf("%s already exists", e)
}

type DeactivatingErr string

func (e DeactivatingErr) Error() string {
    return fmt.Sprintf("%s error while being deactivated", e)
}

type AlreadyDeactivatingErr string

func (e AlreadyDeactivatingErr) Error() string {
    return fmt.Sprintf("%s is already being deactivated", e)
}

