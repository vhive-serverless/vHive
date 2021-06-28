package firecracker

import (
	"context"
	"errors"
	"sync"

	"github.com/ease-lab/vhive/cri"
	"github.com/ease-lab/vhive/ctriface"
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

type FirecrackerService struct {
	sync.Mutex

	stockRuntimeClient criapi.RuntimeServiceClient

	coordinator *coordinator

	// to store mapping from pod to guest image and port temporarily
	podVMConfigs map[string]*VMConfig
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
	guestImage, err := getGuestImage(config)
	if err != nil {
		log.WithError(err).Error()
		return nil, err
	}

	funcInst, err := fs.coordinator.startVM(context.Background(), guestImage)
	if err != nil {
		log.WithError(err).Error("failed to start VM")
		return nil, err
	}

	vmConfig := &VMConfig{guestIP: funcInst.StartVMResponse.GuestIP, guestPort: guestPortValue}
	fs.insertPodVMConfig(r.GetPodSandboxId(), vmConfig)

	// Wait for placeholder UC to be created
	<-stockDone

	containerdID := stockResp.ContainerId
	err = fs.coordinator.insertActive(containerdID, funcInst)
	if err != nil {
		log.WithError(err).Error("failed to insert active VM")
		return nil, err
	}

	return stockResp, stockErr
}

func (fs *FirecrackerService) createQueueProxy(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	vmConfig, err := fs.getPodVMConfig(r.GetPodSandboxId())
	if err != nil {
		log.WithError(err).Error()
		return nil, err
	}

	fs.removePodVMConfig(r.GetPodSandboxId())

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

func (fs *FirecrackerService) insertPodVMConfig(podID string, vmConfig *VMConfig) {
	fs.Lock()
	defer fs.Unlock()

	fs.podVMConfigs[podID] = vmConfig
}

func (fs *FirecrackerService) removePodVMConfig(podID string) {
	fs.Lock()
	defer fs.Unlock()

	delete(fs.podVMConfigs, podID)
}

func (fs *FirecrackerService) getPodVMConfig(podID string) (*VMConfig, error) {
	fs.Lock()
	defer fs.Unlock()

	vmConfig, isPresent := fs.podVMConfigs[podID]
	if !isPresent {
		log.Errorf("VM config for pod %s does not exist", podID)
		return nil, errors.New("VM config for pod does not exist")
	}

	return vmConfig, nil
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
