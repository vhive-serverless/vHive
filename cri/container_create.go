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
		return s.processUserContainer(ctx, r)
	}
	if containerName == queueProxyName {
		return s.processQueueProxy(ctx, r)
	}

	// Containers irrelevant to user's workload
	return s.stockRuntimeClient.CreateContainer(ctx, r)
}

func (s *Service) processUserContainer(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	config := r.GetConfig()

	vmConfig, err := getGuestImagePort(config)
	if err != nil {
		log.WithError(err).Error()
		return nil, err
	}

	s.insertPodConfig(r.GetPodSandboxId(), vmConfig)

	// TODO: (Plamen) Remove when book keeping is fully supported for the VM
	return s.stockRuntimeClient.CreateContainer(ctx, r)
}

func (s *Service) processQueueProxy(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	vmConfig, err := s.getPodConfig(r.GetPodSandboxId())
	if err != nil {
		log.WithError(err).Error()
		return nil, err
	}

	s.removePodConfig(r.GetPodSandboxId())

	fi, err := s.coordinator.startVM(context.Background(), vmConfig.guestImage)
	if err != nil {
		log.WithError(err).Error("failed to start VM")
		return nil, err
	}

	// Add guest IP and port
	guestIPKeyVal := &criapi.KeyValue{Key: guestIPEnv, Value: fi.startVMResponse.GuestIP}
	guestPortKeyVal := &criapi.KeyValue{Key: guestPortEnv, Value: vmConfig.guestPort}
	r.Config.Envs = append(r.Config.Envs, guestIPKeyVal, guestPortKeyVal)

	resp, err := s.stockRuntimeClient.CreateContainer(ctx, r)
	if err != nil {
		log.WithError(err).Error("stock containerd failed to start UC")
		return nil, err
	}

	containerdID := resp.ContainerId
	err = s.coordinator.insertActive(containerdID, fi)
	if err != nil {
		log.WithError(err).Error("failed to insert active VM")
		return nil, err
	}

	return resp, nil
}

func getGuestImagePort(config *criapi.ContainerConfig) (*VMConfig, error) {
	var (
		image, port string
	)

	envs := config.GetEnvs()
	for _, kv := range envs {
		if kv.GetKey() == guestImageEnv {
			image = kv.GetValue()
			break
		}
	}

	// Hardcode port for now
	port = guestPortValue

	if image == "" || port == "" {
		return nil, errors.New("failed to provide non empty guest image and port in user container config")
	}

	return &VMConfig{guestImage: image, guestPort: port}, nil
}
