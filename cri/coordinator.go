// MIT License
//
// Copyright (c) 2020 Plamen Petrov
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

package cri

import (
	"context"
	"errors"
	"strconv"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/ustiugov/fccd-orchestrator/ctriface"
)

const (
	maxVMs = 2000
)

type coordinator struct {
	sync.Mutex
	orch                *ctriface.Orchestrator
	availableIDs        []int
	activeVMs           map[string]string
	withoutOrchestrator bool
}

type coordinatorOption func(*coordinator)

// withoutOrchestrator is used for testing the coordinator without calling the orchestrator
func withoutOrchestrator() coordinatorOption {
	return func(c *coordinator) {
		c.withoutOrchestrator = true
	}
}

func newCoordinator(orch *ctriface.Orchestrator, opts ...coordinatorOption) *coordinator {
	availableIDs := make([]int, maxVMs, maxVMs)
	for i := range availableIDs {
		availableIDs[i] = i
	}

	c := &coordinator{
		availableIDs:        availableIDs,
		activeVMs:           make(map[string]string),
		orch:                orch,
		withoutOrchestrator: false,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *coordinator) startVM(ctx context.Context, image string) (*ctriface.StartVMResponse, string, error) {
	c.Lock()

	vmID := strconv.Itoa(c.reserveID())
	c.Unlock()

	resp, err := c.orchStartVM(ctx, vmID, image)
	if err != nil {
		log.WithError(err).Error("coordinator failed to start VM")
	}

	return resp, vmID, err
}

func (c *coordinator) stopVM(ctx context.Context, containerID string) error {
	c.Lock()
	defer c.Unlock()

	vmID, ok := c.activeVMs[containerID]
	if !ok {
		return nil
	}

	delete(c.activeVMs, containerID)
	c.freeID(vmID)

	return c.orchStopVM(ctx, vmID)

}

func (c *coordinator) insertMapping(containerID, vmID string) error {
	c.Lock()
	defer c.Unlock()

	logger := log.WithFields(log.Fields{"containerID": containerID, "vmID": vmID})

	if _, present := c.activeVMs[containerID]; present {
		logger.Error("entry for container already exists")
		return errors.New("entry for container already exists")
	}

	c.activeVMs[containerID] = vmID
	return nil
}

func (c *coordinator) isActive(containerID string) bool {
	c.Lock()
	defer c.Unlock()

	_, ok := c.activeVMs[containerID]
	return ok
}

func (c *coordinator) reserveID() int {
	id := c.availableIDs[0]
	c.availableIDs = c.availableIDs[1:]

	return id
}

func (c *coordinator) freeID(id string) {
	i, err := strconv.Atoi(id)
	if err != nil {
		log.Panic("provided non-int id")
	}

	c.availableIDs = append(c.availableIDs, i)
}

func (c *coordinator) orchStartVM(ctx context.Context, vmID, image string) (*ctriface.StartVMResponse, error) {
	if c.withoutOrchestrator {
		return nil, nil
	}

	resp, _, err := c.orch.StartVM(ctx, vmID, image)
	return resp, err
}

func (c *coordinator) orchStopVM(ctx context.Context, vmID string) error {
	if c.withoutOrchestrator {
		return nil
	}

	return c.orch.StopSingleVM(ctx, vmID)
}
