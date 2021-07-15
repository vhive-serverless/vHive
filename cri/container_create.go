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

package cri

import (
	"context"
	"errors"

	log "github.com/sirupsen/logrus"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	userContainerName = "user-container"
	queueProxyName    = "queue-proxy"
	guestIPEnv        = "GUEST_ADDR"
	guestPortEnv      = "GUEST_PORT"
	guestImageEnv     = "GUEST_IMAGE"
	guestPortValue    = "50051"
)

// CreateContainer starts a container or a VM, depending on the name
// if the name matches "user-container", the cri plugin starts a VM, assigning it an IP,
// otherwise starts a regular container
func (s *Service) CreateContainer(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	log.Debugf("CreateContainer within sandbox %q for container %+v",
		r.GetPodSandboxId(), r.GetConfig().GetMetadata())

	config := r.GetConfig()
	containerName := config.GetMetadata().GetName()

	if containerName == userContainerName {
		return s.createUserContainer(ctx, r)
	}
	if containerName == queueProxyName {
		return s.createQueueProxy(ctx, r)
	}

	// Containers relevant for control plane
	return s.stockRuntimeClient.CreateContainer(ctx, r)
}

func (s *Service) createUserContainer(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	var (
		stockResp *criapi.CreateContainerResponse
		stockErr  error
		stockDone = make(chan struct{})
	)

	go func() {
		defer close(stockDone)
		stockResp, stockErr = s.stockRuntimeClient.CreateContainer(ctx, r)
	}()

	config := r.GetConfig()
	guestImage, err := getGuestImage(config)
	if err != nil {
		log.WithError(err).Error()
		return nil, err
	}

	funcInst, err := s.coordinator.startVM(context.Background(), guestImage)
	if err != nil {
		log.WithError(err).Error("failed to start VM")
		return nil, err
	}

	vmConfig := &VMConfig{guestIP: funcInst.startVMResponse.GuestIP, guestPort: guestPortValue}
	s.insertPodVMConfig(r.GetPodSandboxId(), vmConfig)

	// Wait for placeholder UC to be created
	<-stockDone

	// Check for error from container creation
	if stockErr != nil {
		log.WithError(stockErr).Error("failed to create container")
		return nil, stockErr
	}
	
	containerdID := stockResp.ContainerId
	err = s.coordinator.insertActive(containerdID, funcInst)
	if err != nil {
		log.WithError(err).Error("failed to insert active VM")
		return nil, err
	}

	return stockResp, stockErr
}

func (s *Service) createQueueProxy(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	vmConfig, err := s.getPodVMConfig(r.GetPodSandboxId())
	if err != nil {
		log.WithError(err).Error()
		return nil, err
	}

	s.removePodVMConfig(r.GetPodSandboxId())

	guestIPKeyVal := &criapi.KeyValue{Key: guestIPEnv, Value: vmConfig.guestIP}
	guestPortKeyVal := &criapi.KeyValue{Key: guestPortEnv, Value: vmConfig.guestPort}
	r.Config.Envs = append(r.Config.Envs, guestIPKeyVal, guestPortKeyVal)

	resp, err := s.stockRuntimeClient.CreateContainer(ctx, r)
	if err != nil {
		log.WithError(err).Error("stock containerd failed to start UC")
		return nil, err
	}

	return resp, nil
}

func getGuestImage(config *criapi.ContainerConfig) (string, error) {
	envs := config.GetEnvs()
	for _, kv := range envs {
		if kv.GetKey() == guestImageEnv {
			return kv.GetValue(), nil
		}

	}

	return "", errors.New("failed to provide non empty guest image in user container config")

}
