// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Plamen Petrov and vHive team
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

package firecracker

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/vhive-serverless/vhive/snapshotting"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/ctriface"

	"github.com/vhive-serverless/vhive/storage"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type coordinator struct {
	sync.Mutex
	orch   *ctriface.Orchestrator
	nextID uint64

	activeInstances     map[string]*funcInstance
	snapshotManager     *snapshotting.SnapshotManager
	withoutOrchestrator bool
}

type coordinatorOption func(*coordinator)

// withoutOrchestrator is used for testing the coordinator without calling the orchestrator
func withoutOrchestrator() coordinatorOption {
	return func(c *coordinator) {
		c.withoutOrchestrator = true
	}
}

func newFirecrackerCoordinator(orch *ctriface.Orchestrator, opts ...coordinatorOption) *coordinator {
	c := &coordinator{
		activeInstances: make(map[string]*funcInstance),
		orch:            orch,
	}

	for _, opt := range opts {
		opt(c)
	}

	snapshotsDir := "/fccd/test/snapshots"
	var objectStore storage.ObjectStorage

	if !c.withoutOrchestrator {
		snapshotsDir = orch.GetSnapshotsDir()
		snapshotsBucket := orch.GetSnapshotsBucket()

		if orch.GetSnapshotMode() == "remote" {
			minioClient, _ := minio.New(orch.GetMinioAddr(), &minio.Options{
				Creds:  credentials.NewStaticV4(orch.GetMinioAccessKey(), orch.GetMinioSecretKey(), ""),
				Secure: false,
			})

			var err error
			objectStore, err = storage.NewMinioStorage(minioClient, snapshotsBucket)
			if err != nil {
				log.WithError(err).Fatalf("failed to create MinIO storage for snapshots in bucket %s", snapshotsBucket)
			}
		}
	}

	c.snapshotManager = snapshotting.NewSnapshotManager(snapshotsDir, objectStore, false)

	return c
}

func (c *coordinator) startVM(ctx context.Context, image, revision string) (*funcInstance, error) {
	return c.startVMWithEnvironment(ctx, image, revision, []string{})
}

func (c *coordinator) startVMWithEnvironment(ctx context.Context, image, revision string, environment []string) (*funcInstance, error) {
	if c.orch != nil && c.orch.GetSnapshotMode() != "disabled" {
		// Check if snapshot is available
		if snap, _ := c.snapshotManager.AcquireSnapshot(revision); snap == nil {
			if c.orch.GetSnapshotMode() == "remote" {
				if exists, _ := c.snapshotManager.SnapshotExists(revision); exists {
					_, _ = c.snapshotManager.DownloadSnapshot(revision)
				}
			}
		}

		if snap, _ := c.snapshotManager.AcquireSnapshot(revision); snap != nil {
			return c.orchLoadInstance(ctx, snap)
		}
	}

	return c.orchStartVM(ctx, image, revision, environment)
}

func (c *coordinator) stopVM(ctx context.Context, containerID string) error {
	c.Lock()

	fi, ok := c.activeInstances[containerID]
	delete(c.activeInstances, containerID)

	c.Unlock()

	if !ok {
		return nil
	}

	if c.orch != nil && c.orch.GetSnapshotMode() != "disabled" && !fi.SnapBooted {
		err := c.orchCreateSnapshot(ctx, fi)
		if err != nil {
			log.Printf("Err creating snapshot %s\n", err)
		}
	}

	return c.orchStopVM(ctx, fi)
}

// for testing
func (c *coordinator) isActive(containerID string) bool {
	c.Lock()
	defer c.Unlock()
	_, ok := c.activeInstances[containerID]
	return ok
}

func (c *coordinator) insertActive(containerID string, fi *funcInstance) error {
	c.Lock()
	defer c.Unlock()

	logger := log.WithFields(log.Fields{"containerID": containerID, "vmID": fi.VmID})

	if fi, present := c.activeInstances[containerID]; present {
		logger.Errorf("entry for container already exists with vmID %s", fi.VmID)
		return errors.New("entry for container already exists")
	}

	c.activeInstances[containerID] = fi
	return nil
}

func (c *coordinator) orchStartVM(ctx context.Context, image, revision string, envVariables []string) (*funcInstance, error) {
	vmID := c.getVMID()
	logger := log.WithFields(
		log.Fields{
			"vmID":     vmID,
			"image":    image,
			"revision": revision,
		},
	)

	logger.Debug("creating fresh instance")

	var (
		resp *ctriface.StartVMResponse
		err  error
	)

	ctxTimeout, cancel := context.WithTimeout(ctx, time.Second*40)
	defer cancel()

	if !c.withoutOrchestrator {
		resp, _, err = c.orch.StartVMWithEnvironment(ctxTimeout, vmID, image, envVariables)
		if err != nil {
			logger.WithError(err).Error("coordinator failed to start VM")
		}
	}

	fi := newFuncInstance(vmID, image, revision, false, resp)
	logger.Debug("successfully created fresh instance")
	return fi, err
}

func (c *coordinator) orchLoadInstance(ctx context.Context, snap *snapshotting.Snapshot) (*funcInstance, error) {
	vmID := c.getVMID()
	logger := log.WithFields(
		log.Fields{
			"vmID":  vmID,
			"image": snap.GetImage(),
		},
	)

	logger.Debug("loading instance from snapshot")

	ctxTimeout, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	resp, _, err := c.orch.LoadSnapshot(ctxTimeout, vmID, snap)
	if err != nil {
		logger.WithError(err).Error("failed to load VM")
		return nil, err
	}

	if _, err := c.orch.ResumeVM(ctxTimeout, vmID); err != nil {
		logger.WithError(err).Error("failed to load VM")
		return nil, err
	}

	fi := newFuncInstance(vmID, snap.GetImage(), snap.GetId(), true, resp)
	logger.Debug("successfully loaded instance from snapshot")
	return fi, nil
}

func (c *coordinator) orchCreateSnapshot(ctx context.Context, fi *funcInstance) error {
	var err error

	snap, err := c.snapshotManager.InitSnapshot(fi.Revision, fi.Image)
	if err != nil {
		fi.Logger.WithError(err).Error("failed to initialize snapshot")
		return nil
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()

	fi.Logger.Debug("creating instance snapshot before stopping")

	if !c.withoutOrchestrator {
		err = c.orch.PauseVM(ctxTimeout, fi.VmID)
		if err != nil {
			fi.Logger.WithError(err).Error("failed to pause VM")
			return err
		}

		err = c.orch.CreateSnapshot(ctxTimeout, fi.VmID, snap)
		if err != nil {
			fi.Logger.WithError(err).Error("failed to create snapshot")
			return err
		}

		if _, err := c.orch.ResumeVM(ctx, fi.VmID); err != nil {
			fi.Logger.WithError(err).Error("failed to resume VM")
			return err
		}
	}

	if err := snap.SerializeSnapInfo(); err != nil {
		fi.Logger.WithError(err).Error("failed to serialize snapshot info")
		return err
	}

	if err := c.snapshotManager.CommitSnapshot(fi.Revision); err != nil {
		fi.Logger.WithError(err).Error("failed to commit snapshot")
		return err
	}

	if !c.withoutOrchestrator && c.orch.GetSnapshotMode() == "remote" {
		if err := c.snapshotManager.UploadSnapshot(fi.Revision); err != nil {
			fi.Logger.WithError(err).Error("failed to upload snapshot")
		}
	}

	return nil
}

func (c *coordinator) orchStopVM(ctx context.Context, fi *funcInstance) error {
	if c.withoutOrchestrator {
		return nil
	}

	if err := c.orch.StopSingleVM(ctx, fi.VmID); err != nil {
		fi.Logger.WithError(err).Error("failed to stop VM for instance")
		return err
	}

	return nil
}

func (c *coordinator) getVMID() string {
	return fmt.Sprintf("%s-%s", strconv.Itoa(int(atomic.AddUint64(&c.nextID, 1))), (uuid.New()).String()[:16])
}
