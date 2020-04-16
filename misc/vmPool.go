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

// Allocate Initializes a VM, activates it and then adds it to VM map
func (p *VMPool) Allocate(vmID string) (*VM, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, isPresent := p.vmMap[vmID]

	if isPresent && vm.isStarting {
		sState := fmt.Sprintf("|S=%t|A=%t|D=%t|", vm.isStarting, vm.isActive, vm.isDeactivating)
		log.WithFields(log.Fields{"vmID": vmID, "state": sState}).Warn("VM is among active VMs")
		return nil, AlreadyStartingErr("VM")
	} else if isPresent {
		panic("allocate VM")
	}

	p.vmMap[vmID] = NewVM(vmID)

	p.vmMap[vmID].setStateStarting()

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

	sState := fmt.Sprintf("|S=%t|A=%t|D=%t|", vm.isStarting, vm.isActive, vm.isDeactivating)
	logger := log.WithFields(log.Fields{"vmID": vmID, "state": sState})

	if !vm.isDeactivating {
		logger.Warn("VM must be in the Deactivating state")
		return DeactivatingErr("VM " + vmID)
	}

	vm.isDeactivating = false // finish lifecycle

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
		sState := fmt.Sprintf("|S=%t|A=%t|D=%t|", vm.isStarting, vm.isActive, vm.isDeactivating)
		s += fmt.Sprintf("vmID=%v, state=%v\n", vmID, sState)
	}

	return s
}

// GetAndDeactivateVM Returns a pointer to the VM
func (p *VMPool) GetAndDeactivateVM(vmID string) (*VM, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	logger := log.WithFields(log.Fields{"vmID": vmID})

	// TODO: VM can be starting and we don't deallocate the VM then
	vm, found := p.vmMap[vmID]
	if !(found && vm.isActive) {
		logger.Warn("VM is not active")
		return nil, NonExistErr("GetAndDeactivateVM")
	}

	vm.setStateDeactivating()

	return vm, nil
}

// GetFuncClient Returns the client to the function
func (p *VMPool) GetFuncClient(vmID string) (*hpb.GreeterClient, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, found := p.vmMap[vmID]
	if !(found && vm.isActive) {
		return nil, NonExistErr("FuncClient")
	}

	return vm.FuncClient, nil
}

// IsVMOff Returns if the VM is shut down
func (p *VMPool) IsVMOff(vmID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, found := p.vmMap[vmID]
	return !found
}

// IsVMStateStarting Returns if the corresponding state is true
func (p *VMPool) IsVMStateStarting(vmID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, found := p.vmMap[vmID]

	return found && vm.isStarting
}

// IsVMStateActive Returns if the corresponding state is true
func (p *VMPool) IsVMStateActive(vmID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, found := p.vmMap[vmID]

	return found && vm.isActive
}

// IsVMStateDeactivating Returns if the corresponding state is true
func (p *VMPool) IsVMStateDeactivating(vmID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, found := p.vmMap[vmID]

	return found && vm.isDeactivating
}

// GetVMStateString Returns VM state description.
func (p *VMPool) GetVMStateString(vmID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, isPresent := p.vmMap[vmID]
	if !isPresent {
		return "NaN"
	}

	return fmt.Sprintf("|S=%t|A=%t|D=%t|", vm.isStarting, vm.isActive, vm.isDeactivating) // TODO: vmID ->fID
}

/*
// SetVMStateStarting Transitions a VM into the corresponding state
func (p *VMPool) SetVMStateStarting(vmID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, isPresent := p.vmMap[vmID]
	if !isPresent {
		return NonExistErr("VM")
	}

	vm.setStateStarting()

	return nil
}
*/

// SetVMStateActive Transitions a VM into the corresponding state
func (p *VMPool) SetVMStateActive(vmID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, isPresent := p.vmMap[vmID]
	if !isPresent {
		return NonExistErr("VM")
	}

	vm.setStateActive()

	return nil
}

/*
// SetVMStateDeactivating Transitions a VM into the corresponding state
func (p *VMPool) SetVMStateDeactivating(vmID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm, isPresent := p.vmMap[vmID]
	if !isPresent {
		return NonExistErr("VM")
	}

	vm.setStateDeactivating()

	return nil
}
*/
