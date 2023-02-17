package networking

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/vishvananda/netns"
	"runtime"
)

type NetworkManager struct {
	// Each VM has a network id. Each ID gets used for the veth pair and generating IP addresses
	freeIDs       []int
	nextID        int
	netConfigs    map[string]NetworkConfig // Maps vmIDs to their network config
	hostIfaceName string
}

// NewOrchestrator Initializes a new orchestrator
func NewNetworkManager() *NetworkManager {
	manager := new(NetworkManager)
	manager.hostIfaceName = "eno49"
	manager.netConfigs = make(map[string]NetworkConfig)
	manager.freeIDs = make([]int, 0)

	startId, err := getNetworkStartID()
	if err == nil {
		manager.nextID = startId
	} else {
		manager.nextID = 0
		fmt.Println(err)
	}

	return manager
}

func (mgr *NetworkManager) createNetConfig(vmID string) {
	var id int
	if len(mgr.freeIDs) == 0 {
		id = mgr.nextID
		mgr.nextID += 1
	} else {
		id = mgr.freeIDs[len(mgr.freeIDs)-1]
		mgr.freeIDs = mgr.freeIDs[:len(mgr.freeIDs)-1]
	}

	mgr.netConfigs[vmID] = NewNetworkConfig(id)
}

func (mgr *NetworkManager) removeNetConfig(vmID string) {
	config := mgr.netConfigs[vmID]
	mgr.freeIDs = append(mgr.freeIDs, config.id)
	delete(mgr.netConfigs, vmID)
}

func (mgr *NetworkManager) CreateNetwork(vmID string) error {
	// Create network config for VM
	mgr.createNetConfig(vmID)
	netCfg := mgr.netConfigs[vmID]

	// Lock the OS Thread so we don't accidentally switch namespaces
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// 0. Get host network namespace
	hostNsHandle, err := netns.Get()
	defer hostNsHandle.Close()
	if err != nil {
		fmt.Printf("Failed to get host ns, %s\n", err)
		return err
	}

	// A. In uVM netns
	// A.1. Create network namespace for uVM & join network namespace
	vmNsHandle, err := netns.NewNamed(netCfg.getNamespaceName()) // Switches namespace
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer vmNsHandle.Close()

	// A.2. Create tap device for uVM
	if err := createTap(netCfg.containerTap, netCfg.gatewayCIDR, netCfg.getNamespaceName()); err != nil {
		return err
	}

	// A.3. Create veth pair for uVM
	// A.3.1 Create veth pair
	if err := createVethPair(netCfg.getVeth0Name(), netCfg.getVeth1Name(), vmNsHandle, hostNsHandle); err != nil {
		return err
	}

	// A.3.2 Configure uVM side veth pair
	if err := configVeth(netCfg.getVeth0Name(), netCfg.getVeth0CIDR()); err != nil {
		return err
	}

	// A.3.3 Designate host side as default gateway for packets leaving namespace
	if err := setDefaultGateway(netCfg.getVeth1CIDR()); err != nil {
		return err
	}

	// A.4. Setup NAT rules
	if err := setupNatRules(netCfg.getVeth0Name(), netCfg.getContainerIP(), netCfg.GetCloneIP()); err != nil {
		return err
	}

	// B. In host netns
	// B.1 Go back to host namespace
	err = netns.Set(hostNsHandle)
	if err != nil {
		return err
	}

	// B.2 Configure host side veth pair
	if err := configVeth(netCfg.getVeth1Name(), netCfg.getVeth1CIDR()); err != nil {
		return err
	}

	// B.3 Add a route on the host for the clone address
	if err := addRoute(netCfg.GetCloneIP(), netCfg.getVeth0CIDR()); err != nil {
		return err
	}

	// B.4 Setup nat to route traffic out of veth device
	if err := setupForwardRules(netCfg.getVeth1Name(), mgr.hostIfaceName); err != nil {
		return err
	}

	return nil
}

func (mgr *NetworkManager) GetConfig(vmID string) *NetworkConfig {
	cfg := mgr.netConfigs[vmID]
	return &cfg
}

func (mgr *NetworkManager) RemoveNetwork(vmID string) error {
	netCfg := mgr.netConfigs[vmID]

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hostNsHandle, err := netns.Get()
	defer hostNsHandle.Close()
	if err != nil {
		fmt.Printf("Failed to get host ns, %s\n", err)
		return err
	}

	// Delete nat to route traffic out of veth device
	if err := deleteForwardRules(netCfg.getVeth1Name(), mgr.hostIfaceName); err != nil {
		return err
	}

	// Delete route on the host for the clone address
	if err := deleteRoute(netCfg.GetCloneIP(), netCfg.getVeth0CIDR()); err != nil {
		return err
	}

	// Get uVM namespace handle
	vmNsHandle, err := netns.GetFromName(netCfg.getNamespaceName())
	defer vmNsHandle.Close()
	if err != nil {
		return err
	}
	err = netns.Set(vmNsHandle)
	if err != nil {
		return err
	}

	// Delete NAT rules
	if err := deleteNatRules(netCfg.getVeth0Name(), netCfg.getContainerIP(), netCfg.GetCloneIP()); err != nil {
		return err
	}

	// Delete default gateway for packets leaving namespace
	if err := deleteDefaultGateway(netCfg.getVeth1CIDR()); err != nil {
		return err
	}

	// Delete uVM side veth pair
	if err := deleteVethPair(netCfg.getVeth0Name(), netCfg.getVeth1Name(), vmNsHandle, hostNsHandle); err != nil {
		return err
	}

	// Delete tap device for uVM
	if err := deleteTap(netCfg.containerTap); err != nil {
		return err
	}

	if err := netns.DeleteNamed(netCfg.getNamespaceName()); err != nil {
		return errors.Wrapf(err, "deleting network namespace")
	}

	err = netns.Set(hostNsHandle)
	if err != nil {
		return err
	}

	mgr.removeNetConfig(vmID)

	return nil
}
