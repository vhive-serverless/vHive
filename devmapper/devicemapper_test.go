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

package devmapper_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	ctrdlog "github.com/containerd/log"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/vhive-serverless/vhive/ctriface/image"
	"github.com/vhive-serverless/vhive/devmapper"
)

const (
	containerdAddress = "/run/firecracker-containerd/containerd.sock"
	NamespaceName     = "containerd"
	TestImageName     = "ghcr.io/ease-lab/helloworld:var_workload"
)

func getAllImages() map[string]string {
	return map[string]string{
		"helloworld":   "ghcr.io/ease-lab/helloworld:var_workload",
		"chameleon":    "ghcr.io/ease-lab/chameleon:var_workload",
		"pyaes":        "ghcr.io/ease-lab/pyaes:var_workload",
		"image_rotate": "ghcr.io/ease-lab/image_rotate:var_workload",
		"lr_training":  "ghcr.io/ease-lab/lr_training:var_workload",
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

func testDevmapper(t *testing.T, mgr *image.ImageManager, dmpr *devmapper.DeviceMapper, snapKey, imageName string) {
	// Pull image
	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), NamespaceName), testTimeout)
	defer cancel()

	img, err := mgr.GetImage(ctx, imageName, true)
	require.NoError(t, err, fmt.Sprintf("Failed to pull image %s", imageName))

	// Test devmapper
	err = dmpr.CreateDeviceSnapshotFromImage(ctx, snapKey, *img)
	require.NoError(t, err, fmt.Sprintf("Failed to create snapshot from image %s", imageName))

	_, err = dmpr.GetDeviceSnapshot(ctx, snapKey)
	if err != nil {
		_ = dmpr.RemoveDeviceSnapshot(ctx, snapKey)
	}
	require.NoError(t, err, fmt.Sprintf("Failed to fetch previously created snapshot %s", snapKey))

	err = dmpr.RemoveDeviceSnapshot(ctx, snapKey)
	require.NoError(t, err, fmt.Sprintf("Failed to remove snapshot %s", snapKey))
}

func TestDevmapper(t *testing.T) {
	snapKey := "testsnap-1"

	// Create containerd client
	client, err := containerd.New(containerdAddress)
	defer func() { _ = client.Close() }()
	require.NoError(t, err, "Containerd client creation returned error")

	// Create image manager
	mgr := image.NewImageManager(client, "devmapper")

	// Create devmapper
	dmpr := devmapper.NewDeviceMapper(client)

	testDevmapper(t, mgr, dmpr, snapKey, TestImageName)
}

func TestDevmapperConcurrent(t *testing.T) {
	// Create containerd client
	client, err := containerd.New(containerdAddress)
	defer func() { _ = client.Close() }()
	require.NoError(t, err, "Containerd client creation returned error")

	// Create image manager
	mgr := image.NewImageManager(client, "devmapper")

	// Create devmapper
	dmpr := devmapper.NewDeviceMapper(client)

	// Test concurrent devmapper
	var wg sync.WaitGroup
	wg.Add(len(getAllImages()))

	for _, imgName := range getAllImages() {
		go func(imgName string) {
			snapKey := fmt.Sprintf("testsnap-%s", imgName)
			testDevmapper(t, mgr, dmpr, snapKey, imgName)
			wg.Done()
		}(imgName)
	}
	wg.Wait()
}
