// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Plamen Petrov, Amory Hoste and vHive team
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

package networking

import (
	"fmt"
	"os"
	"sync"
	"testing"

	ctrdlog "github.com/containerd/log"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	os.Exit(m.Run())
}

func TestCreateCleanManager(t *testing.T) {
	poolSize := []int{1, 5, 20}

	for _, n := range poolSize {
		mgr, createErr := NewNetworkManager("", "", n, "172.17", "172.18")
		require.NoError(t, createErr, "Network manager creation returned error")

		cleanErr := mgr.Cleanup()
		require.NoError(t, cleanErr, "Network manager cleanup returned error")
	}
}

func TestCreateRemoveNetworkParallel(t *testing.T) {
	netNum := []int{50, 200, 1500}

	mgr, err := NewNetworkManager("", "", 10, "172.17", "172.18")
	require.NoError(t, err, "Network manager creation returned error")
	defer func() { _ = mgr.Cleanup() }()

	for _, n := range netNum {
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, err := mgr.CreateNetwork(fmt.Sprintf("func_%d", i))
				require.NoError(t, err, fmt.Sprintf("Failed to create network for func_%d", i))
			}(i)
		}
		wg.Wait()
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				err := mgr.RemoveNetwork(fmt.Sprintf("func_%d", i))
				require.NoError(t, err, fmt.Sprintf("Failed to remove network for func_%d", i))
			}(i)
		}
		wg.Wait()
	}
}

func TestCreateRemoveNetworkSerial(t *testing.T) {
	netNum := 50

	mgr, err := NewNetworkManager("", "", 50, "172.17", "172.18")
	require.NoError(t, err, "Network manager creation returned error")
	defer func() { _ = mgr.Cleanup() }()

	for i := 0; i < netNum; i++ {
		_, err = mgr.CreateNetwork(fmt.Sprintf("func_%d", i))
		require.NoError(t, err, "Failed to create network")
	}

	for i := 0; i < netNum; i++ {
		err = mgr.RemoveNetwork(fmt.Sprintf("func_%d", i))
		require.NoError(t, err, "Failed to remove network")
	}
}
