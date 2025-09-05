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

	//	"fmt"

	log "github.com/sirupsen/logrus"
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
	id        string
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
	// var (
	// 	stockResp *criapi.CreateContainerResponse
	// 	stockErr  error
	// 	stockDone = make(chan struct{})
	// )

	//	go func() {
	//		defer close(stockDone)
	//		stockResp, stockErr = fs.stockRuntimeClient.CreateContainer(ctx, r)
	//	}()

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

	environment := cri.ToStringArray(config.GetEnvs())
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

	vmConfig := &VMConfig{id: funcInst.VmID, guestIP: funcInst.StartVMResponse.GuestIP, guestPort: guestPort}
	fs.insertVMConfig(r.GetPodSandboxId(), vmConfig)

	// Wait for placeholder UC to be created
	//	<-stockDone

	// Check for error from container creation
	//	if stockErr != nil {
	//		log.WithError(stockErr).Error("failed to create container")
	//		return nil, stockErr
	//	}

	containerdID := funcInst.VmID
	// err = fs.coordinator.insertActive(containerdID, funcInst)
	// if err != nil {
	// 	log.WithError(err).Error("failed to insert active VM")
	// 	return nil, err
	// }

	return &criapi.CreateContainerResponse{ContainerId: containerdID}, err
}

func (fs *FirecrackerService) createQueueProxy(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	vmConfig, err := fs.getVMConfig(r.GetPodSandboxId())
	if err != nil {
		log.WithError(err).Error()
		return nil, err
	}

	// fs.removeVMConfig(r.GetPodSandboxId())

	guestIPKeyVal := &criapi.KeyValue{Key: guestIPEnv, Value: vmConfig.guestIP}
	guestPortKeyVal := &criapi.KeyValue{Key: guestPortEnv, Value: vmConfig.guestPort}
	//	readinessProbeKeyVal := &criapi.KeyValue{Key: "SERVING_READINESS_PROBE", Value: ""}//fmt.Sprintf("{\"tcpSocket\":{\"port\":50051,\"host\":\"%s\"},\"successThreshold\":1}", vmConfig.guestIP)}
	// for _, e := range r.Config.Envs {
	// 	if e.Key == "SERVING_READINESS_PROBE" {
	// 		e.Value = ""
	// 		log.Debug("changed the readiness probe")
	// 	}
	// }
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

// StartContainer starts the container.
func (fs *FirecrackerService) StartContainer(ctx context.Context, r *criapi.StartContainerRequest) (*criapi.StartContainerResponse, error) {

	containerID := r.GetContainerId()

	log.Debug(fs.vmConfigs)
	// if we have the vm with this id, we should ignore the start because it is running already
	if _, err := fs.findVMConfigByID(containerID); err == nil {
		log.Infof("Ignoring StartContainer for %q because the VM is already running", r.GetContainerId())
		return &criapi.StartContainerResponse{}, nil
	}

	log.Debugf("StartContainer for %q", r.GetContainerId())
	return fs.stockRuntimeClient.StartContainer(ctx, r)
}

func (fs *FirecrackerService) insertVMConfig(podID string, vmConfig *VMConfig) {
	fs.Lock()
	defer fs.Unlock()

	fs.vmConfigs[podID] = vmConfig
}

func (fs *FirecrackerService) findVMConfigByID(vmID string) (*VMConfig, error) {
	fs.Lock()
	defer fs.Unlock()

	for _, vmConfig := range fs.vmConfigs {
		if vmConfig.id == vmID {
			return vmConfig, nil
		}
	}

	return nil, errors.New("VM config for ID does not exist")
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
