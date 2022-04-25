// MIT License
//
// Copyright (c) 2020 Plamen Petrov, Amory Hoste and EASE lab
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

package image

import (
	"context"
	"fmt"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"os"
	"sync"
	"testing"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const (
	TestImageName = "ghcr.io/ease-lab/helloworld:var_workload"
	containerdAddress      = "/run/firecracker-containerd/containerd.sock"
	NamespaceName          = "containerd"
)

func getAllImages() map[string]string {
	return map[string]string{
		"helloworld":          "ghcr.io/ease-lab/helloworld:var_workload",
		"chameleon":           "ghcr.io/ease-lab/chameleon:var_workload",
		"pyaes":               "ghcr.io/ease-lab/pyaes:var_workload",
		"image_rotate":        "ghcr.io/ease-lab/image_rotate:var_workload",
		"lr_training":         "ghcr.io/ease-lab/lr_training:var_workload",
	}
}

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	os.Exit(m.Run())
}

func TestSingleConcurrent(t *testing.T) {
	// Create client
	client, err := containerd.New(containerdAddress)
	defer func() { _ = client.Close() }()
	require.NoError(t, err, "Containerd client creation returned error")

	// Create image manager
	mgr := NewImageManager(client, "devmapper")

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), NamespaceName), testTimeout)
	defer cancel()

	// Pull image
	var wg sync.WaitGroup
	concurrentPulls := 100
	wg.Add(concurrentPulls)

	for i := 0; i < concurrentPulls; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := mgr.GetImage(ctx, TestImageName)
			require.NoError(t, err, fmt.Sprintf("Failed to pull image %s", TestImageName))
		}(i)
	}
	wg.Wait()
}

func TestMultipleConcurrent(t *testing.T) {
	// Create client
	client, err := containerd.New(containerdAddress)
	defer func() { _ = client.Close() }()
	require.NoError(t, err, "Containerd client creation returned error")

	// Create image manager
	mgr := NewImageManager(client, "devmapper")

	testTimeout := 300 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), NamespaceName), testTimeout)
	defer cancel()

	// Pull image
	var wg sync.WaitGroup
	concurrentPulls := 100
	wg.Add(len(getAllImages()))

	for _, imgName := range getAllImages() {
		go func(imgName string) {
			var imgWg sync.WaitGroup
			imgWg.Add(concurrentPulls)
			for i := 0; i < concurrentPulls; i++ {
				go func(i int) {
					defer imgWg.Done()
					_, err := mgr.GetImage(ctx, imgName)
					require.NoError(t, err, fmt.Sprintf("Failed to pull image %s", imgName))
				}(i)
			}
			imgWg.Wait()
			wg.Done()
		}(imgName)
	}

	wg.Wait()
}