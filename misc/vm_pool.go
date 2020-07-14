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
	"github.com/ustiugov/fccd-orchestrator/taps"
)

// NewVMPool Initializes a pool of VMs
func NewVMPool(niNum int) *VMPool {
	p := new(VMPool)
	p.tapManager = taps.NewTapManager()

	return p
}

// Allocate Initializes a VM, activates it and then adds it to VM map
func (p *VMPool) Allocate(vmID string) (*VM, error) {

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Allocating a VM instance")

	if _, isPresent := p.vmMap.Load(vmID); isPresent {
		logger.Panic("Allocate (VM): VM exists in the map")
	}

	vm := NewVM(vmID)

	var err error
	vm.Ni, err = p.tapManager.AddTap(vmID + "_tap")
	if err != nil {
		logger.Warn("Ni allocation failed, freeing VM from the pool")
		return nil, err
	}

	p.vmMap.Store(vmID, vm)

	return vm, nil
}

// Free Removes a VM from the pool and transitions it to Deactivating
func (p *VMPool) Free(vmID string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Freeing a VM instance")

	_, isPresent := p.vmMap.Load(vmID)
	if !isPresent {
		log.WithFields(log.Fields{"vmID": vmID}).Panic("Free (VM): VM does not exist in the map")
		return NonExistErr("Free (VM): VM does not exist when freeing a VM from the pool")
	}

	logger.Debug("Free (VM): Freeing VM from the pool")

	if err := p.tapManager.RemoveTap(vmID + "_tap"); err != nil {
		logger.Error("Could not delete tap")
		return err
	}

	p.vmMap.Delete(vmID)

	return nil
}

// GetVMMap Returns a copy of vmMap as a regular concurrency-unsafe map
func (p *VMPool) GetVMMap() map[string]*VM {
	m := make(map[string]*VM)
	p.vmMap.Range(func(key, value interface{}) bool {
		m[key.(string)] = value.(*VM)
		return true
	})

	return m
}

// GetVM Returns a pointer to the VM
func (p *VMPool) GetVM(vmID string) (*VM, error) {
	vm, found := p.vmMap.Load(vmID)
	if !found {
		log.WithFields(log.Fields{"vmID": vmID}).Panic("VM is not in the VM map")
		return nil, NonExistErr("GetVM: VM is not in the VM map")
	}

	return vm.(*VM), nil
}

// GetFuncClient Returns the client to the function
func (p *VMPool) GetFuncClient(vmID string) (*hpb.GreeterClient, error) {
	vm, found := p.vmMap.Load(vmID)
	if !found {
		log.WithFields(log.Fields{"vmID": vmID}).Panic("GetFuncClient: VM is not in the VM map")
		return nil, NonExistErr("GetFuncClient: VM is not in the VM map")
	}

	return vm.(*VM).FuncClient, nil
}

// RemoveBridges Removes the bridges created by the tap manager
func (p *VMPool) RemoveBridges() {
	p.tapManager.RemoveBridges()
}
