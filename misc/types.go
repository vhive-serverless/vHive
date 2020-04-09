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
    "log"
    "sync"

    "github.com/containerd/containerd"
    "google.golang.org/grpc"
    "github.com/pkg/errors"

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
    vm := new(VM)
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

func (p *NiPool) Allocate() (*NetworkInterface, error) {
    var ni NetworkInterface
    p.mu.Lock()
    defer p.mu.Unlock()
    if len(p.niList) == 0 {
        return nil, errors.New("No NI available")
    }
    ni, p.niList = p.niList[0], p.niList[1:]

    return &ni, nil
}

func (p *NiPool) Free(ni *NetworkInterface) {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.niList = append(p.niList, *ni)
}

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

func (p *VmPool) Allocate(vmID string) (*VM, error) {
    p.mu.Lock()
    defer p.mu.Unlock()

    vm_, isPresent := p.vmMap[vmID]
    if isPresent && vm_.isStarting == true {
        log.Printf("VM %v is among active VMs", vmID)
        return nil, AlreadyStartingErr("VM")
    } else if isPresent {
        panic("allocate VM")
    }

    vm := NewVM(vmID)
    p.vmMap[vmID] = *vm

    return vm, nil
}

func (p *VmPool) Free(vmID string) (VM, error) {
    p.mu.Lock()
    defer p.mu.Unlock()

    vm, isPresent := p.vmMap[vmID]
    if !isPresent {
        return vm, AlreadyDeactivatingErr("VM " + vmID)
    }

    if p.IsVmActive(vmID) == false && vm.isDeactivating == true {
        log.Printf("VM %v is among active VMs but already being deactivated", vmID)
        return vm, AlreadyDeactivatingErr("VM " + vmID)
    } else if p.IsVmActive(vmID) == false {
        log.Printf("WARNING: VM %v is inactive when trying to deallocate, do nothing", vmID)
        return vm, DeactivatingErr("VM " + vmID)
    }

    vm.SetStateDeactivating()
    delete(p.vmMap, vmID)

    return vm, nil
}

func (p *VmPool) GetVmMap() (map[string]VM) {
    return p.vmMap
}

func (p *VmPool) IsVmActive(vmID string) (bool) {
    vm, isPresent := p.vmMap[vmID]
    return isPresent && vm.isActive
}

func (p *VmPool) GetFuncClient(vmID string) (*hpb.GreeterClient, error) {
    p.mu.Lock() // can be replaced by a per-VM lock?
    defer p.mu.Unlock()

    if !p.IsVmActive(vmID) {
        return nil, NonExistErr("FuncClient")
    }

    vm, _ := p.vmMap[vmID]

    return vm.FuncClient, nil
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

