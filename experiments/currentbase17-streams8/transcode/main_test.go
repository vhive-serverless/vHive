package main

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPageCountDoesNotRequireSortedPFNs(t *testing.T) {
	n, err := pageCount([]byte("pfn\n9\n2\n7\n"))
	require.NoError(t, err)
	require.Equal(t, 3, n)
	for _, s := range []string{"pfn\n2\n2\n", "pfn\n-1\n", "hash\n2\n", "pfn\n2,3\n"} {
		_, err := pageCount([]byte(s))
		require.Error(t, err)
	}
}
