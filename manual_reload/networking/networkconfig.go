package networking

import (
	"fmt"
	"net"
)

// Can at most have 2^14 VMs at the same time
type NetworkConfig struct {
	id            int
	containerCIDR string // Container IP address (CIDR notation)
	gatewayCIDR   string // Container gateway IP address
	containerTap  string // Container tap name
}

func NewNetworkConfig(id int) NetworkConfig {
	return NetworkConfig{id: id, containerCIDR: "172.16.0.2/24", gatewayCIDR: "172.16.0.1/24", containerTap: "tap0"}
}

func (cfg *NetworkConfig) getVeth0Name() string {
	return fmt.Sprintf("veth%d-0", cfg.id)
}

func (cfg *NetworkConfig) getVeth0CIDR() string {
	return fmt.Sprintf("172.17.%d.%d/30", (4*cfg.id)/256, ((4*cfg.id)+2)%256)
}

func (cfg *NetworkConfig) getVeth1Name() string {
	return fmt.Sprintf("veth%d-1", cfg.id)
}

func (cfg *NetworkConfig) getVeth1CIDR() string {
	return fmt.Sprintf("172.17.%d.%d/30", (4*cfg.id)/256, ((4*cfg.id)+1)%256)
}

func (cfg *NetworkConfig) GetCloneIP() string {
	return fmt.Sprintf("172.18.%d.%d", cfg.id/254, 1+(cfg.id%254))
}

func (cfg *NetworkConfig) getNamespaceName() string {
	return fmt.Sprintf("uvmns%d", cfg.id)
}
func (cfg *NetworkConfig) getContainerIP() string {
	ip, _, _ := net.ParseCIDR(cfg.containerCIDR)
	return ip.String()
}

func (cfg *NetworkConfig) GetGatewayIP() string {
	ip, _, _ := net.ParseCIDR(cfg.gatewayCIDR)
	return ip.String()
}

func (cfg *NetworkConfig) GetNamespacePath() string {
	return fmt.Sprintf("/var/run/netns/%s", cfg.getNamespaceName())
}

func (cfg *NetworkConfig) GetContainerCIDR() string {
	return cfg.containerCIDR
}
