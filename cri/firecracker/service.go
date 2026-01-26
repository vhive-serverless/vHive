// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Plamen Petrov, Nathaniel Tornow and vHive team
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
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/common"
	"github.com/vhive-serverless/vhive/cri"
	"github.com/vhive-serverless/vhive/ctriface"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	userContainerName = "user-container"
	queueProxyName    = "queue-proxy"
	guestIPEnv        = "GUEST_ADDR"
	guestPortEnv      = "GUEST_PORT"
	guestImageEnv     = "GUEST_IMAGE"
	revisionEnv       = "K_REVISION"
)

type FirecrackerService struct {
	sync.Mutex

	stockRuntimeClient criapi.RuntimeServiceClient

	coordinator *coordinator

	vmConfigs map[string]*VMConfig
}

// VMConfig wraps the IP and port of the guest VM
type VMConfig struct {
	guestIP   string
	guestPort string
}

func NewFirecrackerService(orch *ctriface.Orchestrator) (*FirecrackerService, error) {
	fs := new(FirecrackerService)
	stockRuntimeClient, err := cri.NewStockRuntimeServiceClient()
	if err != nil {
		log.WithError(err).Error("failed to create new stock runtime service client")
		return nil, err
	}
	fs.stockRuntimeClient = stockRuntimeClient
	fs.coordinator = newFirecrackerCoordinator(orch)
	fs.vmConfigs = make(map[string]*VMConfig)
	return fs, nil
}

// CreateContainer starts a container or a VM, depending on the name
// if the name matches "user-container", the cri plugin starts a VM, assigning it an IP,
// otherwise starts a regular container
func (s *FirecrackerService) CreateContainer(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
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

func (fs *FirecrackerService) createUserContainer(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	var (
		stockResp *criapi.CreateContainerResponse
		stockErr  error
		stockDone = make(chan struct{})
	)

	go func() {
		defer close(stockDone)
		stockResp, stockErr = fs.stockRuntimeClient.CreateContainer(ctx, r)
	}()

	config := r.GetConfig()
	guestImage, err := getEnvVal(guestImageEnv, config)
	if err != nil {
		log.WithError(err).Error()
		return nil, err
	}

	revision, err := getEnvVal(revisionEnv, config)
	if err != nil {
		log.WithError(err).Error()
		return nil, err
	}

	environment := common.ToStringArray(config.GetEnvs())
	funcInst, err := fs.coordinator.startVMWithEnvironment(context.Background(), guestImage, revision, environment)
	if err != nil {
		log.WithError(err).Error("failed to start VM")
		return nil, err
	}

	guestPort, err := getEnvVal(guestPortEnv, config)
	if err != nil {
		log.WithError(err).Error()
		return nil, err
	}

	vmConfig := &VMConfig{guestIP: funcInst.StartVMResponse.GuestIP, guestPort: guestPort}
	fs.insertVMConfig(r.GetPodSandboxId(), vmConfig)

	// Wait for placeholder UC to be created
	<-stockDone

	// Check for error from container creation
	if stockErr != nil {
		log.WithError(stockErr).Error("failed to create container")
		return nil, stockErr
	}

	containerdID := stockResp.ContainerId
	err = fs.coordinator.insertActive(containerdID, funcInst)
	if err != nil {
		log.WithError(err).Error("failed to insert active VM")
		return nil, err
	}

	return stockResp, stockErr
}

func (fs *FirecrackerService) createQueueProxy(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	vmConfig, err := fs.getVMConfig(r.GetPodSandboxId())
	if err != nil {
		log.WithError(err).Error()
		return nil, err
	}

	fs.removeVMConfig(r.GetPodSandboxId())

	guestIPKeyVal := &criapi.KeyValue{Key: guestIPEnv, Value: vmConfig.guestIP}
	guestPortKeyVal := &criapi.KeyValue{Key: guestPortEnv, Value: vmConfig.guestPort}
	r.Config.Envs = append(r.Config.Envs, guestIPKeyVal, guestPortKeyVal)

	resp, err := fs.stockRuntimeClient.CreateContainer(ctx, r)
	if err != nil {
		log.WithError(err).Error("stock containerd failed to start UC")
		return nil, err
	}

	return resp, nil
}

func (fs *FirecrackerService) RemoveContainer(ctx context.Context, r *criapi.RemoveContainerRequest) (*criapi.RemoveContainerResponse, error) {
	log.Debugf("RemoveContainer for %q", r.GetContainerId())
	containerID := r.GetContainerId()

	go func() {
		if err := fs.coordinator.stopVM(context.Background(), containerID); err != nil {
			log.WithError(err).Error("failed to stop microVM")
		}
	}()

	return fs.stockRuntimeClient.RemoveContainer(ctx, r)
}

func (fs *FirecrackerService) insertVMConfig(podID string, vmConfig *VMConfig) {
	fs.Lock()
	defer fs.Unlock()

	fs.vmConfigs[podID] = vmConfig
}

func (fs *FirecrackerService) removeVMConfig(podID string) {
	fs.Lock()
	defer fs.Unlock()

	delete(fs.vmConfigs, podID)
}

func (fs *FirecrackerService) getVMConfig(podID string) (*VMConfig, error) {
	fs.Lock()
	defer fs.Unlock()

	vmConfig, isPresent := fs.vmConfigs[podID]
	if !isPresent {
		log.Errorf("VM config for pod %s does not exist", podID)
		return nil, errors.New("VM config for pod does not exist")
	}

	return vmConfig, nil
}

func getEnvVal(key string, config *criapi.ContainerConfig) (string, error) {
	envs := config.GetEnvs()
	for _, kv := range envs {
		if kv.GetKey() == key {
			return kv.GetValue(), nil
		}

	}

	return "", errors.New("failed to retrieve environment variable from user container config")
}
