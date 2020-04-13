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

	log "github.com/sirupsen/logrus"

	hpb "github.com/ustiugov/fccd-orchestrator/helloworld"
)

// NewVMPool Initializes a pool of VMs
func NewVMPool() *VMPool {
	p := new(VMPool)
	p.mu = &sync.Mutex{}
	p.vmMap = make(map[string]VM)

	return p
}

// Allocate Initializes a VM and adds it to the pool
func (p *VMPool) Allocate(vmID string) (*VM, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	vmTmp, isPresent := p.vmMap[vmID]
	if isPresent && vmTmp.isStarting {
		log.Printf("VM %v is among active VMs", vmID)
		return nil, AlreadyStartingErr("VM")
	} else if isPresent {
		panic("allocate VM")
	}

	vm := NewVM(vmID)
	p.vmMap[vmID] = *vm

	return vm, nil
}

// Free Removes a VM from the pool and transitions it to Deactivating
func (p *VMPool) Free(vmID string) (VM, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, isPresent := p.vmMap[vmID]
	if !isPresent {
		return vm, AlreadyDeactivatingErr("VM " + vmID)
	}

	if !p.IsVMActive(vmID) && vm.isDeactivating {
		log.Printf("VM %v is among active VMs but already being deactivated", vmID)
		return vm, AlreadyDeactivatingErr("VM " + vmID)
	} else if !p.IsVMActive(vmID) {
		log.Printf("WARNING: VM %v is inactive when trying to deallocate, do nothing", vmID)
		return vm, DeactivatingErr("VM " + vmID)
	}

	vm.SetStateDeactivating()
	delete(p.vmMap, vmID)

	return vm, nil
}

// GetVMMap Returns the map of VMs
func (p *VMPool) GetVMMap() map[string]VM {
	return p.vmMap
}

// IsVMActive Returns if the VM is active (in the active state and in the map)
func (p *VMPool) IsVMActive(vmID string) bool {
	vm, isPresent := p.vmMap[vmID]
	return isPresent && vm.isActive
}

// GetFuncClient Returns the client to the function
func (p *VMPool) GetFuncClient(vmID string) (*hpb.GreeterClient, error) {
	p.mu.Lock() // can be replaced by a per-VM lock?
	defer p.mu.Unlock()

	if !p.IsVMActive(vmID) {
		return nil, NonExistErr("FuncClient")
	}

	vm := p.vmMap[vmID]

	return vm.FuncClient, nil
}
