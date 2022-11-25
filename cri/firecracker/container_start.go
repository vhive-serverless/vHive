package firecracker

import (
	"context"
	"github.com/containerd/containerd/namespaces"
	log "github.com/sirupsen/logrus"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// StartContainer starts the container.
func (fs *FirecrackerService) StartContainer(ctx context.Context, r *criapi.StartContainerRequest) (retRes *criapi.StartContainerResponse, retErr error) {
	ctx = namespaces.WithNamespace(ctx, "k8s.io")
	containerId := r.GetContainerId()

	cntr, err := fs.firecrackerContainerdClient.LoadContainer(ctx, containerId)
	if err != nil {
		log.WithError(err).Errorf("Could not load container with id %s\n", containerId)
	}

	log.Infof("Loaded container %+v\n", cntr)
	return &criapi.StartContainerResponse{}, nil
}
