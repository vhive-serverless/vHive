// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Amory Hoste and vHive team
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

// Package networking provides primitives to connect function instances to the network.
package networking

import (
	"sync"

	log "github.com/sirupsen/logrus"
)

// NetworkManager manages the in use network configurations along with a pool of free network configurations
// that can be used to connect a function instance to the network.
type NetworkManager struct {
	sync.Mutex
	nextID        int
	hostIfaceName string
	vethPrefix    string
	clonePrefix   string

	// Pool of free network configs
	networkPool []*NetworkConfig
	poolCond    *sync.Cond
	poolSize    int

	// Mapping of function instance IDs to their network config
	netConfigs map[string]*NetworkConfig

	// Network configs that are being created
	inCreation sync.WaitGroup
}

// NewNetworkManager creates and returns a new network manager that connects function instances to the network
// using the supplied interface. If no interface is supplied, the default interface is used. To take the network
// setup of the critical path of a function creation, the network manager tries to maintain a pool of ready to use
// network configurations of size at least poolSize.
func NewNetworkManager(hostIfaceName string, poolSize int, vethPrefix, clonePrefix string) (*NetworkManager, error) {
	manager := new(NetworkManager)

	manager.hostIfaceName = hostIfaceName
	if manager.hostIfaceName == "" {
		hostIface, err := getHostIfaceName()
		if err != nil {
			return nil, err
		} else {
			manager.hostIfaceName = hostIface
		}
	}

	manager.netConfigs = make(map[string]*NetworkConfig)
	manager.networkPool = make([]*NetworkConfig, 0)

	startId, err := getNetworkStartID()
	if err == nil {
		manager.nextID = startId
	} else {
		manager.nextID = 0
	}

	manager.poolCond = sync.NewCond(new(sync.Mutex))
	manager.vethPrefix = vethPrefix
	manager.clonePrefix = clonePrefix
	manager.initConfigPool(poolSize)
	manager.poolSize = poolSize

	return manager, nil
}

// initConfigPool fills an empty network pool up to the given poolSize
func (mgr *NetworkManager) initConfigPool(poolSize int) {
	var wg sync.WaitGroup
	wg.Add(poolSize)

	logger := log.WithFields(log.Fields{"poolSize": poolSize})
	logger.Debug("Initializing network pool")

	// Concurrently create poolSize network configs
	for i := 0; i < poolSize; i++ {
		go func() {
			mgr.addNetConfig()
			wg.Done()
		}()
	}
	wg.Wait()
}

// addNetConfig creates and initializes a new network config
func (mgr *NetworkManager) addNetConfig() {
	mgr.Lock()
	id := mgr.nextID
	mgr.nextID += 1
	mgr.inCreation.Add(1)
	mgr.Unlock()

	netCfg := NewNetworkConfig(id, mgr.hostIfaceName, mgr.vethPrefix, mgr.clonePrefix)
	if err := netCfg.CreateNetwork(); err != nil {
		log.Errorf("failed to create network %s:", err)
	}

	mgr.poolCond.L.Lock()
	mgr.networkPool = append(mgr.networkPool, netCfg)
	// Signal in case someone is waiting for a new config to become available in the pool
	mgr.poolCond.Signal()
	mgr.poolCond.L.Unlock()
	mgr.inCreation.Done()
}

// allocNetConfig allocates a new network config from the pool to a function instance identified by funcID
func (mgr *NetworkManager) allocNetConfig(funcID string) *NetworkConfig {
	logger := log.WithFields(log.Fields{"funcID": funcID})
	logger.Debug("Allocating a new network config from network pool to function instance")

	mgr.poolCond.L.Lock()
	// Add netconfig to pool to keep pool to configured size
	if len(mgr.networkPool) <= mgr.poolSize {
		go mgr.addNetConfig()
	}
	for len(mgr.networkPool) == 0 {
		// Wait until a new network config has been created
		mgr.poolCond.Wait()
	}

	// Pop a network config from the pool and allocate it to the function instance
	config := mgr.networkPool[len(mgr.networkPool)-1]
	mgr.networkPool = mgr.networkPool[:len(mgr.networkPool)-1]
	mgr.poolCond.L.Unlock()

	mgr.Lock()
	mgr.netConfigs[funcID] = config
	mgr.Unlock()

	logger = log.WithFields(log.Fields{
		"funcID":        funcID,
		"ContainerIP":   config.getContainerIP(),
		"NamespaceName": config.getNamespaceName(),
		"Veth0CIDR":     config.getVeth0CIDR(),
		"Veth0Name":     config.getVeth0Name(),
		"Veth1CIDR":     config.getVeth1CIDR(),
		"Veth1Name":     config.getVeth1Name(),
		"CloneIP":       config.GetCloneIP(),
		"ContainerCIDR": config.GetContainerCIDR(),
		"GatewayIP":     config.GetGatewayIP(),
		"HostDevName":   config.GetHostDevName(),
		"NamespacePath": config.GetNamespacePath()})

	logger.Debug("Allocated a new network config")

	return config
}

// releaseNetConfig releases the network config of a given function instance with id funcID back to the pool
func (mgr *NetworkManager) releaseNetConfig(funcID string) {
	logger := log.WithFields(log.Fields{"funcID": funcID})

	mgr.Lock()
	config, ok := mgr.netConfigs[funcID]
	if !ok {
		mgr.Unlock()
		logger.Warn("failed to find network config for function")
		return
	}
	delete(mgr.netConfigs, funcID)
	mgr.Unlock()

	logger.Debug("Releasing network config from function instance and adding it to network pool")

	// Add network config back to the pool. We allow the pool to grow over it's configured size here since the
	// overhead of keeping a network config in the pool is low compared to the cost of creating a new config.
	mgr.poolCond.L.Lock()
	mgr.networkPool = append(mgr.networkPool, config)
	mgr.poolCond.Signal()
	mgr.poolCond.L.Unlock()
}

// CreateNetwork creates the networking for a function instance identified by funcID
func (mgr *NetworkManager) CreateNetwork(funcID string) (*NetworkConfig, error) {
	logger := log.WithFields(log.Fields{"funcID": funcID})
	logger.Debug("Creating network config for function instance")

	netCfg := mgr.allocNetConfig(funcID)
	return netCfg, nil
}

// GetConfig returns the network config assigned to a function instance identified by funcID
func (mgr *NetworkManager) GetConfig(funcID string) *NetworkConfig {
	mgr.Lock()
	defer mgr.Unlock()

	cfg := mgr.netConfigs[funcID]
	return cfg
}

// RemoveNetwork removes the network config of a function instance identified by funcID. The allocated network devices
// for the given function instance must not be in use anymore when calling this function.
func (mgr *NetworkManager) RemoveNetwork(funcID string) error {
	logger := log.WithFields(log.Fields{"funcID": funcID})
	logger.Debug("Removing network config for function instance")
	mgr.releaseNetConfig(funcID)
	return nil
}

// Cleanup removes and deallocates all network configurations that are in use or in the network pool. Make sure to first
// clean up all running functions before removing their network configs.
func (mgr *NetworkManager) Cleanup() error {
	log.Info("Cleaning up network manager")
	mgr.Lock()
	defer mgr.Unlock()

	// Wait till all network configs still in creation are added
	mgr.inCreation.Wait()

	// Release network configs still in use
	var wgu sync.WaitGroup
	wgu.Add(len(mgr.netConfigs))
	for funcID := range mgr.netConfigs {
		config := mgr.netConfigs[funcID]
		go func(config *NetworkConfig) {
			if err := config.RemoveNetwork(); err != nil {
				log.Errorf("failed to remove network %s:", err)
			}
			wgu.Done()
		}(config)
	}
	wgu.Wait()
	mgr.netConfigs = make(map[string]*NetworkConfig)

	// Cleanup network pool
	mgr.poolCond.L.Lock()
	var wg sync.WaitGroup
	wg.Add(len(mgr.networkPool))

	for _, config := range mgr.networkPool {
		go func(config *NetworkConfig) {
			if err := config.RemoveNetwork(); err != nil {
				log.Errorf("failed to remove network %s:", err)
			}
			wg.Done()
		}(config)
	}
	wg.Wait()
	mgr.networkPool = make([]*NetworkConfig, 0)
	mgr.poolCond.L.Unlock()

	return nil
}
