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

	log "github.com/sirupsen/logrus"

	hpb "github.com/ustiugov/fccd-orchestrator/helloworld"
)

// NewVMPool Initializes a pool of VMs
func NewVMPool() *VMPool {
	p := new(VMPool)
	p.mu = &sync.Mutex{}
	p.vmMap = make(map[string]*VM)

	return p
}

// Allocate Initializes a VM and adds it to the pool
func (p *VMPool) Allocate(vmID string) (*VM, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	vmTmp, isPresent := p.vmMap[vmID]

	if isPresent && vmTmp.isStarting {
		log.WithFields(log.Fields{"vmID": vmID, "state": vmTmp.GetVMStateString()}).Warn("VM is among active VMs")
		return nil, AlreadyStartingErr("VM")
	} else if isPresent {
		panic("allocate VM")
	}

	p.vmMap[vmID] = NewVM(vmID)

	return p.vmMap[vmID], nil
}

// Free Removes a VM from the pool and transitions it to Deactivating
func (p *VMPool) Free(vmID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, isPresent := p.vmMap[vmID]
	if !isPresent {
		log.WithFields(log.Fields{"vmID": vmID}).Warn("VM does not exist in the VM map")
		return DeactivatingErr("VM " + vmID)
	}

	logger := log.WithFields(log.Fields{"vmID": vmID, "state": vm.GetVMStateString()})

	if !vm.isDeactivating {
		logger.Warn("VM must be in the Deactivating state")
		return DeactivatingErr("VM " + vmID)
	}

	delete(p.vmMap, vmID)

	return nil
}

// GetVMMap Returns the map of VMs
func (p *VMPool) GetVMMap() map[string]*VM {
	return p.vmMap
}

// SprintVMMap Returns a string with VMs' ID and state list
func (p *VMPool) SprintVMMap() (s string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for vmID, vm := range p.vmMap {
		s += fmt.Sprintf("vmID=%v, state=%v\n", vmID, vm.GetVMStateString())
	}

	return s
}

// GetVM Returns a pointer to the VM
func (p *VMPool) GetVM(vmID string) (*VM, error) {
	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Acquiring the lock")
	p.mu.Lock() // can be replaced by a per-VM lock?
	defer p.mu.Unlock()
	logger.Debug("Acquired the lock")

	if !p.IsVMStateActive(vmID) {
		return nil, NonExistErr("VM")
	}

	vm := p.vmMap[vmID]

	return vm, nil
}

// GetFuncClient Returns the client to the function
func (p *VMPool) GetFuncClient(vmID string) (*hpb.GreeterClient, error) {
	p.mu.Lock() // can be replaced by a per-VM lock?
	defer p.mu.Unlock()

	if !p.IsVMStateActive(vmID) {
		return nil, NonExistErr("FuncClient")
	}

	vm := p.vmMap[vmID]

	return vm.FuncClient, nil
}

// IsVMOff Returns if the VM is shut down
func (p *VMPool) IsVMOff(vmID string) bool {
	_, found := p.vmMap[vmID]
	return !found
}

// IsVMStateStarting Returns if the corresponding state is true
func (p *VMPool) IsVMStateStarting(vmID string) bool {
	p.mu.Lock() // can be replaced by a per-VM lock?
	defer p.mu.Unlock()

	vm, found := p.vmMap[vmID]

	return found && vm.isStarting
}

// IsVMStateActive Returns if the corresponding state is true
func (p *VMPool) IsVMStateActive(vmID string) bool {
	p.mu.Lock() // can be replaced by a per-VM lock?
	defer p.mu.Unlock()

	vm, found := p.vmMap[vmID]

	return found && vm.isActive
}

// IsVMStateDeactivating Returns if the corresponding state is true
func (p *VMPool) IsVMStateDeactivating(vmID string) bool {
	p.mu.Lock() // can be replaced by a per-VM lock?
	defer p.mu.Unlock()

	vm, found := p.vmMap[vmID]

	return found && vm.isDeactivating
}

/*
// SetStateStarting Transitions a VM into the corresponding state
func (p *VMPool) SetStateStarting(vmID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, isPresent := p.vmMap[vmID]
	if !isPresent {
		return NonExistErr("VM")
	}

	vm.SetStateStarting()

	return nil
}

// SetStateActive Transitions a VM into the corresponding state
func (p *VMPool) SetStateActive(vmID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, isPresent := p.vmMap[vmID]
	if !isPresent {
		return NonExistErr("VM")
	}

	vm.SetStateStarting()

	return nil
}

// SetStateDeactivating Transitions a VM into the corresponding state
func (p *VMPool) SetStateDeactivating(vmID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, isPresent := p.vmMap[vmID]
	if !isPresent {
		return NonExistErr("VM")
	}

	vm.SetStateDeactivating()

	return nil
}
*/
