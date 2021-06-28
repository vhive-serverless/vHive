package cri

import (
	"context"

	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

type ServiceInterface interface {
	CreateContainer(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error)
	RemoveContainer(ctx context.Context, r *criapi.RemoveContainerRequest) (*criapi.RemoveContainerResponse, error)
}
