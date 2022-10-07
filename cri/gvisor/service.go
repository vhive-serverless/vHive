// MIT License
//
// Copyright (c) 2020 Nathaniel Tornow and EASE lab
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

package gvisor

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/vhive-serverless/vhive/cri"
	log "github.com/sirupsen/logrus"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	userContainerName = "user-container"
	queueProxyName    = "queue-proxy"
	guestIPEnv        = "GUEST_ADDR"
	guestPortEnv      = "GUEST_PORT"
	guestImageEnv     = "GUEST_IMAGE"
)

type GVisorService struct {
	sync.Mutex
	stockRuntimeClient criapi.RuntimeServiceClient
	coor               *coordinator

	// maps the pod to the IP-address of the according gvisor-UserContainer
	podIDToCtrConf map[string]*ctrConfig
}

type ctrConfig struct {
	guestIP   string
	guestPort string
}

func NewGVisorService() (*GVisorService, error) {
	gs := new(GVisorService)
	stockRC, err := cri.NewStockRuntimeServiceClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create new stock runtime service client: %v", err)
	}
	gs.stockRuntimeClient = stockRC
	coor, err := newCoordinator()
	if err != nil {
		return nil, fmt.Errorf("failed to create gvisor-coordinator: %v", err)
	}
	gs.coor = coor
	gs.podIDToCtrConf = make(map[string]*ctrConfig)
	return gs, nil
}

func (gs *GVisorService) CreateContainer(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	log.Debugf("CreateContainer within sandbox %q for container %+v",
		r.GetPodSandboxId(), r.GetConfig().GetMetadata())

	config := r.GetConfig()
	containerName := config.GetMetadata().GetName()

	if containerName == userContainerName {
		return gs.createUserContainer(ctx, r)
	}
	if containerName == queueProxyName {
		return gs.createQueueProxy(ctx, r)
	}

	// Containers relevant for control plane
	return gs.stockRuntimeClient.CreateContainer(ctx, r)
}

func (gs *GVisorService) createUserContainer(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	var (
		stockResp *criapi.CreateContainerResponse
		stockErr  error
		stockDone = make(chan struct{})
	)

	go func() {
		defer close(stockDone)
		stockResp, stockErr = gs.stockRuntimeClient.CreateContainer(ctx, r)
	}()

	config := r.GetConfig()
	guestImage, err := getEnvVal(guestImageEnv, config)
	if err != nil {
		log.WithError(err).Error()
		return nil, err
	}

	ctr, err := gs.coor.startContainer(ctx, guestImage)
	if err != nil {
		log.WithError(err).Error("failed to start container")
		return nil, err
	}

	guestPort, err := getEnvVal(guestPortEnv, config)
	if err != nil {
		log.WithError(err).Error()
		return nil, err
	}

	ctrConfig := &ctrConfig{guestIP: ctr.ip, guestPort: guestPort}
	gs.insertCtrConfig(r.GetPodSandboxId(), ctrConfig)

	<-stockDone
	gs.coor.insertActive(stockResp.GetContainerId(), ctr)
	return stockResp, stockErr
}

func (gs *GVisorService) createQueueProxy(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	ctrConf, err := gs.getAndRemoveCtrConfig(r.GetPodSandboxId())
	if err != nil {
		log.WithError(err).Error()
		return nil, err
	}

	guestIPKeyVal := &criapi.KeyValue{Key: guestIPEnv, Value: ctrConf.guestIP}
	guestPortKeyVal := &criapi.KeyValue{Key: guestPortEnv, Value: ctrConf.guestPort}
	r.Config.Envs = append(r.Config.Envs, guestIPKeyVal, guestPortKeyVal)

	resp, err := gs.stockRuntimeClient.CreateContainer(ctx, r)
	if err != nil {
		log.WithError(err).Error("stock containerd failed to start UC")
		return nil, err
	}

	return resp, nil
}

func (gs *GVisorService) RemoveContainer(ctx context.Context, r *criapi.RemoveContainerRequest) (*criapi.RemoveContainerResponse, error) {
	log.Debugf("RemoveContainer for %q", r.GetContainerId())
	containerID := r.GetContainerId()

	go func() {
		if err := gs.coor.stopContainer(ctx, containerID); err != nil {
			log.WithError(err).Error("failed to stop container")
		}
	}()
	return gs.stockRuntimeClient.RemoveContainer(ctx, r)
}

func (gs *GVisorService) insertCtrConfig(podID string, ctrConf *ctrConfig) {
	gs.Lock()
	defer gs.Unlock()
	gs.podIDToCtrConf[podID] = ctrConf
}

func (gs *GVisorService) getAndRemoveCtrConfig(podID string) (*ctrConfig, error) {
	gs.Lock()
	defer gs.Unlock()

	ctrConf, ok := gs.podIDToCtrConf[podID]
	if !ok {
		return nil, fmt.Errorf("no UC-ip for this pod present")
	}
	delete(gs.podIDToCtrConf, podID)
	return ctrConf, nil
}

func getEnvVal(key string, config *criapi.ContainerConfig) (string, error) {
	envs := config.GetEnvs()
	for _, kv := range envs {
		if kv.GetKey() == key {
			return kv.GetValue(), nil
		}

	}

	return "", errors.New("failed to provide non empty guest image in user container config")

}
