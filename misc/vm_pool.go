// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov, Plamen Petrov and EASE lab
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
	"github.com/ease-lab/vhive/networking"
	"github.com/ease-lab/vhive/taps"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// NewVMPool Initializes a pool of VMs
func NewVMPool(hostIface string, netPoolSize int, isFullLocal bool) *VMPool {
	p := new(VMPool)
	p.isFullLocal = isFullLocal

	var err error
	if p.isFullLocal {
		p.networkManager, err = networking.NewNetworkManager(hostIface, netPoolSize)
	} else {
		p.tapManager, err = taps.NewTapManager(hostIface)
	}
	if err != nil {
		log.Println(err)
	}

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
	if p.isFullLocal {
		logger.Debug("Create virtual network for VM")
		vm.netConfig, err = p.networkManager.CreateNetwork(vmID)
	} else {
		vm.ni, err = p.tapManager.AddTap(vmID+"_tap")
	}

	if err != nil {
		logger.Warn("VM network creation failed")
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
		logger.Warn("VM does not exist in the map")
		return nil
	}

	if p.isFullLocal {
		if err := p.networkManager.RemoveNetwork(vmID); err != nil {
			logger.Error("Could not remove network config")
			return err
		}
	} else {
		if err := p.tapManager.RemoveTap(vmID + "_tap"); err != nil {
			logger.Error("Could not delete tap")
			return err
		}
	}

	p.vmMap.Delete(vmID)

	return nil
}

// RecreateTap Deletes and creates the tap for a VM
func (p *VMPool) RecreateTap(vmID string) error {
	if p.isFullLocal {
		return errors.New("RecreateTap is not supported for full local snapshots")
	}

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Recreating tap")

	_, isPresent := p.vmMap.Load(vmID)
	if !isPresent {
		log.WithFields(log.Fields{"vmID": vmID}).Panic("RecreateTap: VM does not exist in the map")
		return NonExistErr("RecreateTap: VM does not exist when recreating its tap")
	}

	if err := p.tapManager.RemoveTap(vmID + "_tap"); err != nil {
		logger.Error("Failed to delete tap")
		return err
	}

	_, err := p.tapManager.AddTap(vmID+"_tap")
	if err != nil {
		logger.Error("Failed to add tap")
		return err
	}

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
		log.WithFields(log.Fields{"vmID": vmID}).Error("VM is not in the VM map")
		return nil, NonExistErr("GetVM: VM is not in the VM map")
	}

	return vm.(*VM), nil
}

// CleanupNetwork removes and deallocates all network configurations
func (p *VMPool) CleanupNetwork() {
	if p.isFullLocal {
		_ = p.networkManager.Cleanup()
	} else {
		p.tapManager.RemoveBridges()
	}
}
