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
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vhive-serverless/vhive/ctriface"
	log "github.com/sirupsen/logrus"
)

type coordinator struct {
	sync.Mutex
	orch   *ctriface.Orchestrator
	nextID uint64

	activeInstances     map[string]*funcInstance
	idleInstances       map[string][]*funcInstance
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
		idleInstances:   make(map[string][]*funcInstance),
		orch:            orch,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *coordinator) getIdleInstance(image string) *funcInstance {
	c.Lock()
	defer c.Unlock()

	idles, ok := c.idleInstances[image]
	if !ok {
		c.idleInstances[image] = []*funcInstance{}
		return nil
	}

	if len(idles) != 0 {
		fi := idles[0]
		c.idleInstances[image] = idles[1:]
		return fi
	}

	return nil
}

func (c *coordinator) setIdleInstance(fi *funcInstance) {
	c.Lock()
	defer c.Unlock()

	_, ok := c.idleInstances[fi.Image]
	if !ok {
		c.idleInstances[fi.Image] = []*funcInstance{}
	}

	c.idleInstances[fi.Image] = append(c.idleInstances[fi.Image], fi)
}

func (c *coordinator) startVM(ctx context.Context, image string) (*funcInstance, error) {
	if fi := c.getIdleInstance(image); c.orch != nil && c.orch.GetSnapshotsEnabled() && fi != nil {
		err := c.orchLoadInstance(ctx, fi)
		return fi, err
	}

	return c.orchStartVM(ctx, image)
}

func (c *coordinator) stopVM(ctx context.Context, containerID string) error {
	c.Lock()

	fi, ok := c.activeInstances[containerID]
	delete(c.activeInstances, containerID)

	c.Unlock()

	if !ok {
		return nil
	}

	if c.orch != nil && c.orch.GetSnapshotsEnabled() {
		return c.orchOffloadInstance(ctx, fi)
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
		logger.Errorf("entry for container already exists with vmID %s" + fi.VmID)
		return errors.New("entry for container already exists")
	}

	c.activeInstances[containerID] = fi
	return nil
}

func (c *coordinator) orchStartVM(ctx context.Context, image string) (*funcInstance, error) {
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
		resp, _, err = c.orch.StartVM(ctxTimeout, vmID, image)
		if err != nil {
			logger.WithError(err).Error("coordinator failed to start VM")
		}
	}

	fi := newFuncInstance(vmID, image, resp)
	logger.Debug("successfully created fresh instance")
	return fi, err
}

func (c *coordinator) orchLoadInstance(ctx context.Context, fi *funcInstance) error {
	fi.Logger.Debug("found idle instance to load")

	ctxTimeout, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	if _, err := c.orch.LoadSnapshot(ctxTimeout, fi.VmID); err != nil {
		fi.Logger.WithError(err).Error("failed to load VM")
		return err
	}

	if _, err := c.orch.ResumeVM(ctxTimeout, fi.VmID); err != nil {
		fi.Logger.WithError(err).Error("failed to load VM")
		return err
	}

	fi.Logger.Debug("successfully loaded idle instance")
	return nil
}

func (c *coordinator) orchCreateSnapshot(ctx context.Context, fi *funcInstance) error {
	var err error

	fi.OnceCreateSnapInstance.Do(
		func() {
			ctxTimeout, cancel := context.WithTimeout(ctx, time.Second*60)
			defer cancel()

			fi.Logger.Debug("creating instance snapshot on first time offloading")

			err = c.orch.PauseVM(ctxTimeout, fi.VmID)
			if err != nil {
				fi.Logger.WithError(err).Error("failed to pause VM")
				return
			}

			err = c.orch.CreateSnapshot(ctxTimeout, fi.VmID)
			if err != nil {
				fi.Logger.WithError(err).Error("failed to create snapshot")
				return
			}
		},
	)

	return err
}

func (c *coordinator) orchOffloadInstance(ctx context.Context, fi *funcInstance) error {
	fi.Logger.Debug("offloading instance")

	if err := c.orchCreateSnapshot(ctx, fi); err != nil {
		return err
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	if err := c.orch.Offload(ctxTimeout, fi.VmID); err != nil {
		fi.Logger.WithError(err).Error("failed to offload instance")
	}

	c.setIdleInstance(fi)

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
