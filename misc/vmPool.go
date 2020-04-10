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
    "log"

    hpb "github.com/ustiugov/fccd-orchestrator/helloworld"
)

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
