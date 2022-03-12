// MIT License
//
// Copyright (c) 2020 Plamen Petrov and EASE lab
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
	"fmt"
	"github.com/ease-lab/vhive/ctriface"
	"github.com/ease-lab/vhive/metrics"
	"github.com/ease-lab/vhive/snapshotting"
	"github.com/ease-lab/vhive/snapshotting/fulllocal"
	"github.com/ease-lab/vhive/snapshotting/regular"
	"github.com/pkg/errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
)

const snapshotsDir = "/fccd/snapshots"

type coordinator struct {
	sync.Mutex
	orch   *ctriface.Orchestrator
	nextID uint64
	isSparseSnaps bool
	isFullLocal bool

	activeInstances     map[string]*FuncInstance
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

func newFirecrackerCoordinator(
	orch *ctriface.Orchestrator,
	snapsCapacityMiB int64,
	isSparseSnaps bool,
	isFullLocal bool,
	opts ...coordinatorOption) *coordinator {

	c := &coordinator{
		activeInstances: make(map[string]*FuncInstance),
		orch:            orch,
		isSparseSnaps:   isSparseSnaps,
		isFullLocal: isFullLocal,
	}

	if isFullLocal {
		c.snapshotManager = snapshotting.NewSnapshotManager(fulllocal.NewSnapshotManager(snapshotsDir, snapsCapacityMiB))
	} else {
		c.snapshotManager = snapshotting.NewSnapshotManager(regular.NewSnapshotManager(snapshotsDir))
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *coordinator) startVM(ctx context.Context, image string, revision string, memSizeMib, vCPUCount uint32) (*FuncInstance, error) {
	if c.orch != nil && c.orch.GetSnapshotsEnabled()  {
		id := image
		if c.isFullLocal {
			id = revision
		}

		// Check if snapshot is available
		if snap, err := c.snapshotManager.AcquireSnapshot(id); err == nil {
			if snap.MemSizeMib != memSizeMib || snap.VCPUCount != vCPUCount {
				return nil, errors.New("uVM memory size or vCPU count in the snapshot do not match the requested ones.")
			}

			vmID := ""
			if c.isFullLocal {
				vmID = strconv.Itoa(int(atomic.AddUint64(&c.nextID, 1)))
			} else {
				vmID = snap.GetId()
			}

			return c.orchStartVMSnapshot(ctx, snap, memSizeMib, vCPUCount, vmID)

		}
	}

	return c.orchStartVM(ctx, image, revision, memSizeMib, vCPUCount)
}

func (c *coordinator) stopVM(ctx context.Context, containerID string) error {
	c.Lock()

	funcInst, present := c.activeInstances[containerID]
	if present {
		delete(c.activeInstances, containerID)
	}

	c.Unlock()

	// Not a request to remove vm container
	if !present {
		return nil
	}

	if c.orch == nil || ! c.orch.GetSnapshotsEnabled() {
		return c.orchStopVM(ctx, funcInst)
	}

	id := funcInst.vmID
	if c.isFullLocal {
		id = funcInst.revisionId
	}

	if funcInst.snapBooted {
		// Release snapshot after the VM has been stopped / offloaded
		defer func() { _ = c.snapshotManager.ReleaseSnapshot(id) }()
	} else {
		// Create snapshot
		err := c.orchCreateSnapshot(ctx, funcInst)
		if err != nil {
			log.Printf("Err creating snapshot %s\n", err)
		}
	}

	if c.isFullLocal {
		return c.orchStopVM(ctx, funcInst)
	} else {
		return c.orchOffloadVM(ctx, funcInst)
	}
}

// for testing
func (c *coordinator) isActive(containerID string) bool {
	c.Lock()
	defer c.Unlock()

	_, ok := c.activeInstances[containerID]
	return ok
}

func (c *coordinator) insertActive(containerID string, funcInst *FuncInstance) error {
	c.Lock()
	defer c.Unlock()

	logger := log.WithFields(log.Fields{"containerID": containerID, "vmID": funcInst.vmID})

	if fi, present := c.activeInstances[containerID]; present {
		logger.Errorf("entry for container already exists with vmID %s" + fi.vmID)
		return errors.New("entry for container already exists")
	}

	c.activeInstances[containerID] = funcInst
	return nil
}

func (c *coordinator) orchStartVM(ctx context.Context, image, revision string, memSizeMib, vCPUCount uint32) (*FuncInstance, error) {
	tStartCold := time.Now()
	vmID := strconv.Itoa(int(atomic.AddUint64(&c.nextID, 1)))
	logger := log.WithFields(
		log.Fields{
			"vmID":  vmID,
			"image": image,
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
		trackDirtyPages := c.isSparseSnaps
		resp, _, err = c.orch.StartVM(ctxTimeout, vmID, image, memSizeMib, vCPUCount, trackDirtyPages)
		if err != nil {
			logger.WithError(err).Error("coordinator failed to start VM")
		}
	}

	coldStartTimeMs := metrics.ToMs(time.Since(tStartCold))

	funcInst := NewFuncInstance(vmID, image, revision, resp, false, memSizeMib, vCPUCount, coldStartTimeMs)
	logger.Debug("successfully created fresh instance")
	return funcInst, err
}

func (c *coordinator) orchStartVMSnapshot(
	ctx context.Context,
	snap *snapshotting.Snapshot,
	memSizeMib,
	vCPUCount uint32,
	vmID string) (*FuncInstance, error) {

	tStartCold := time.Now()
	logger := log.WithFields(
		log.Fields{
			"vmID":  vmID,
			"image": snap.GetImage(),
		},
	)

	logger.Debug("loading instance from snapshot")

	var (
		resp *ctriface.StartVMResponse
		err  error
	)

	ctxTimeout, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	resp, _, err = c.orch.LoadSnapshot(ctxTimeout, vmID, snap)
	if err != nil {
		logger.WithError(err).Error("failed to load VM")
		return nil, err
	}

	if _, err := c.orch.ResumeVM(ctxTimeout, vmID); err != nil {
		logger.WithError(err).Error("failed to load VM")
		return nil, err
	}

	coldStartTimeMs := metrics.ToMs(time.Since(tStartCold))
	funcInst := NewFuncInstance(vmID, snap.GetImage(), snap.GetId(), resp, true, memSizeMib, vCPUCount, coldStartTimeMs)
	logger.Debug("successfully loaded instance from snapshot")

	return funcInst, err
}

func (c *coordinator) orchCreateSnapshot(ctx context.Context, funcInst *FuncInstance) error {
	logger := log.WithFields(
		log.Fields{
			"vmID":  funcInst.vmID,
			"image": funcInst.image,
		},
	)

	id := funcInst.vmID
	if c.isFullLocal {
		id = funcInst.revisionId
	}

	_, snap, err := c.snapshotManager.InitSnapshot(
		id,
		funcInst.image,
		funcInst.coldStartTimeMs,
		funcInst.memSizeMib,
		funcInst.vCPUCount,
		c.isSparseSnaps)

	if err != nil {
		funcInst.logger.Warn(fmt.Sprint(err))
		return nil
	}

	// This call is only necessary if the alternative approach in devicemapper with thin-delta is used.
	/*if c.isFullLocal && removeContainerSnaps != nil {
		for _, cleanupSnapId := range *removeContainerSnaps {
			if err := c.orch.CleanupSnapshot(ctx, cleanupSnapId); err != nil {
				return errors.Wrap(err, "removing devmapper revision snapshot")
			}
		}
	}*/

	ctxTimeout, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()

	logger.Debug("creating instance snapshot before stopping")

	err = c.orch.PauseVM(ctxTimeout, funcInst.vmID)
	if err != nil {
		logger.WithError(err).Error("failed to pause VM")
		return nil
	}

	err = c.orch.CreateSnapshot(ctxTimeout, funcInst.vmID, snap)
	if err != nil {
		funcInst.logger.WithError(err).Error("failed to create snapshot")
		return nil
	}

	// TODO: StopVM does not work for fullLocal snapshots without resuming. Might be the same for offloaded since
	// those are never stopped
	if c.isFullLocal {
		_, err = c.orch.ResumeVM(ctx, funcInst.vmID)
	}

	if err := c.snapshotManager.CommitSnapshot(id); err != nil {
		funcInst.logger.WithError(err).Error("failed to commit snapshot")
		return err
	}

	return nil
}

func (c *coordinator) orchOffloadVM(ctx context.Context, funcInst *FuncInstance) error {
	if c.withoutOrchestrator {
		return nil
	}

	if err := c.orch.OffloadVM(ctx, funcInst.vmID); err != nil {
		funcInst.logger.WithError(err).Error("failed to offload VM")
		return err
	}

	return nil
}

func (c *coordinator) orchStopVM(ctx context.Context, funcInst *FuncInstance) error {
	if c.withoutOrchestrator {
		return nil
	}

	if err := c.orch.StopSingleVM(ctx, funcInst.vmID); err != nil {
		funcInst.logger.WithError(err).Error("failed to stop VM for instance")
		return err
	}

	return nil
}
