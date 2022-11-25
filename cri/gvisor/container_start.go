package gvisor

import (
	"context"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// StartContainer starts the container.
func (g *GVisorService) StartContainer(ctx context.Context, r *criapi.StartContainerRequest) (retRes *criapi.StartContainerResponse, retErr error) {
	return nil, nil
}
