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
	log "github.com/sirupsen/logrus"

	hpb "github.com/ustiugov/fccd-orchestrator/helloworld"
)

// NewVMPool Initializes a pool of VMs
func NewVMPool(niNum int) *VMPool {
	p := new(VMPool)
	p.vmMap = make(map[string]*VM)
	p.niPool = NewNiPool(niNum)

	return p
}

// Allocate Initializes a VM, activates it and then adds it to VM map
func (p *VMPool) Allocate(vmID string) (*VM, error) {
	p.Lock()
	defer p.Unlock()

	logger := log.WithFields(log.Fields{"vmID": vmID})
	if _, isPresent := p.vmMap[vmID]; isPresent {
		logger.Panic("Allocate (VM): VM exists in the map")
	}

	p.vmMap[vmID] = NewVM(vmID)

	var err error
	p.vmMap[vmID].Ni, err = p.niPool.Allocate()
	if err != nil {
		logger.Warn("Ni allocation failed, freeing VM from the pool")
		delete(p.vmMap, vmID)
		return nil, err
	}

	return p.vmMap[vmID], nil
}

// Free Removes a VM from the pool and transitions it to Deactivating
func (p *VMPool) Free(vmID string) error {
	p.Lock()
	defer p.Unlock()

	logger := log.WithFields(log.Fields{"vmID": vmID})

	vm, isPresent := p.vmMap[vmID]
	if !isPresent {
		log.WithFields(log.Fields{"vmID": vmID}).Panic("Free (VM): VM does not exist in the map")
		return NonExistErr("Free (VM): VM does not exist when freeing a VM from the pool")
	}

	logger.Debug("Free (VM): Freeing VM from the pool")

	p.niPool.Free(vm.Ni)

	delete(p.vmMap, vmID)

	return nil
}

// GetVMMap Returns the map of VMs
func (p *VMPool) GetVMMap() map[string]*VM {
	p.RLock()
	defer p.RUnlock()

	return p.vmMap
}

// GetVM Returns a pointer to the VM
func (p *VMPool) GetVM(vmID string) (*VM, error) {
	p.RLock()
	defer p.RUnlock()

	vm, found := p.vmMap[vmID]
	if !found {
		log.WithFields(log.Fields{"vmID": vmID}).Panic("VM is not in the VM map")
		return nil, NonExistErr("GetVM: VM is not in the VM map")
	}

	return vm, nil
}

// GetFuncClient Returns the client to the function
func (p *VMPool) GetFuncClient(vmID string) (*hpb.GreeterClient, error) {
	p.RLock()
	defer p.RUnlock()

	vm, found := p.vmMap[vmID]
	if !found {
		log.WithFields(log.Fields{"vmID": vmID}).Panic("GetFuncClient: VM is not in the VM map")
		return nil, NonExistErr("GetFuncClient: VM is not in the VM map")
	}

	return vm.FuncClient, nil
}
