package networking

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsolatedNetworkNames(t *testing.T) {
	old := NewNetworkConfig(123, "eth0", "172.26", "172.27")
	current := NewNetworkConfig(123, "eth0", "172.30", "172.31", "sstr")
	require.Equal(t, "uvmns123", old.getNamespaceName())
	require.Equal(t, "veth123-1", old.getVeth1Name())
	require.Equal(t, "/var/run/netns/sstruvmns123", current.GetNamespacePath())
	require.Equal(t, "sstrveth123-0", current.getVeth0Name())
	require.Equal(t, "sstrveth123-1", current.getVeth1Name())
	require.NotEqual(t, old.GetCloneIP(), current.GetCloneIP())
	require.Equal(t, old.GetContainerCIDR(), current.GetContainerCIDR())
	maxID := NewNetworkConfig(16383, "eth0", "172.30", "172.31", "sstr")
	require.LessOrEqual(t, len(maxID.getVeth1Name()), 15)
}
