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

	log "github.com/sirupsen/logrus"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// RunPodSandbox creates and starts a pod-level sandbox. Runtimes must ensure
// the sandbox is in the ready state on success.
func (s *Service) RunPodSandbox(ctx context.Context, r *criapi.RunPodSandboxRequest) (*criapi.RunPodSandboxResponse, error) {
	log.Debugf("RunPodsandbox for %+v", r.GetConfig().GetMetadata())
	return s.stockRuntimeClient.RunPodSandbox(ctx, r)
}

// ListPodSandbox returns a list of PodSandboxes.
func (s *Service) ListPodSandbox(ctx context.Context, r *criapi.ListPodSandboxRequest) (*criapi.ListPodSandboxResponse, error) {
	log.Tracef("ListPodSandbox with filter %+v", r.GetFilter())
	return s.stockRuntimeClient.ListPodSandbox(ctx, r)
}

// PodSandboxStatus returns the status of the PodSandbox. If the PodSandbox is not
// present, returns an error.
func (s *Service) PodSandboxStatus(ctx context.Context, r *criapi.PodSandboxStatusRequest) (*criapi.PodSandboxStatusResponse, error) {
	log.Tracef("PodSandboxStatus for %q", r.GetPodSandboxId())
	return s.stockRuntimeClient.PodSandboxStatus(ctx, r)
}

// StopPodSandbox stops any running process that is part of the sandbox and
// reclaims network resources (e.g., IP addresses) allocated to the sandbox.
func (s *Service) StopPodSandbox(ctx context.Context, r *criapi.StopPodSandboxRequest) (*criapi.StopPodSandboxResponse, error) {
	log.Debugf("StopPodSandbox for %q", r.GetPodSandboxId())
	return s.stockRuntimeClient.StopPodSandbox(ctx, r)
}

// RemovePodSandbox removes the sandbox. If there are any running containers
// in the sandbox, they must be forcibly terminated and removed.
func (s *Service) RemovePodSandbox(ctx context.Context, r *criapi.RemovePodSandboxRequest) (*criapi.RemovePodSandboxResponse, error) {
	log.Debugf("RemovePodSandbox for %q", r.GetPodSandboxId())
	return s.stockRuntimeClient.RemovePodSandbox(ctx, r)

}

// PortForward prepares a streaming endpoint to forward ports from a PodSandbox.
func (s *Service) PortForward(ctx context.Context, r *criapi.PortForwardRequest) (*criapi.PortForwardResponse, error) {
	log.Debugf("Portforward for %q port %v", r.GetPodSandboxId(), r.GetPort())
	return s.stockRuntimeClient.PortForward(ctx, r)
}

// StartContainer starts the container.
func (s *Service) StartContainer(ctx context.Context, r *criapi.StartContainerRequest) (*criapi.StartContainerResponse, error) {
	log.Debugf("StartContainer for %q", r.GetContainerId())
	return s.stockRuntimeClient.StartContainer(ctx, r)

}

// ListContainers lists all containers by filters.
func (s *Service) ListContainers(ctx context.Context, r *criapi.ListContainersRequest) (*criapi.ListContainersResponse, error) {
	log.Tracef("ListContainers with filter %+v", r.GetFilter())
	return s.stockRuntimeClient.ListContainers(ctx, r)
}

// ContainerStatus returns status of the container. If the container is not
// present, returns an error.
func (s *Service) ContainerStatus(ctx context.Context, r *criapi.ContainerStatusRequest) (*criapi.ContainerStatusResponse, error) {
	log.Tracef("ContainerStatus for %q", r.GetContainerId())
	return s.stockRuntimeClient.ContainerStatus(ctx, r)
}

// StopContainer stops a running container with a grace period (i.e., timeout).
func (s *Service) StopContainer(ctx context.Context, r *criapi.StopContainerRequest) (*criapi.StopContainerResponse, error) {
	log.Debugf("StopContainer for %q with timeout %d (s)", r.GetContainerId(), r.GetTimeout())
	return s.stockRuntimeClient.StopContainer(ctx, r)
}

// ExecSync runs a command in a container synchronously.
func (s *Service) ExecSync(ctx context.Context, r *criapi.ExecSyncRequest) (*criapi.ExecSyncResponse, error) {
	log.Debugf("ExecSync for %q with command %+v and timeout %d (s)", r.GetContainerId(), r.GetCmd(), r.GetTimeout())
	return s.stockRuntimeClient.ExecSync(ctx, r)
}

// Exec prepares a streaming endpoint to execute a command in the container.
func (s *Service) Exec(ctx context.Context, r *criapi.ExecRequest) (*criapi.ExecResponse, error) {
	log.Debugf("Exec for %v", r)
	return s.stockRuntimeClient.Exec(ctx, r)
}

// Attach prepares a streaming endpoint to attach to a running container.
func (s *Service) Attach(ctx context.Context, r *criapi.AttachRequest) (*criapi.AttachResponse, error) {
	log.Debugf("Attach for %q with tty %v and stdin %v", r.GetContainerId(), r.GetTty(), r.GetStdin())
	return s.stockRuntimeClient.Attach(ctx, r)
}

// UpdateContainerResources updates ContainerConfig of the container.
func (s *Service) UpdateContainerResources(ctx context.Context, r *criapi.UpdateContainerResourcesRequest) (*criapi.UpdateContainerResourcesResponse, error) {
	log.Debugf("UpdateContainerResources for %q with %+v", r.GetContainerId(), r.GetLinux())
	return s.stockRuntimeClient.UpdateContainerResources(ctx, r)
}

// PullImage pulls an image with authentication config.
func (s *Service) PullImage(ctx context.Context, r *criapi.PullImageRequest) (*criapi.PullImageResponse, error) {
	log.Debugf("PullImage %q", r.GetImage().GetImage())
	return s.stockImageClient.PullImage(ctx, r)
}

// ListImages lists existing images.
func (s *Service) ListImages(ctx context.Context, r *criapi.ListImagesRequest) (*criapi.ListImagesResponse, error) {
	log.Tracef("ListImages with filter %+v", r.GetFilter())
	return s.stockImageClient.ListImages(ctx, r)
}

// ImageStatus returns the status of the image. If the image is not
// present, returns a response with ImageStatusResponse.Image set to
// nil.
func (s *Service) ImageStatus(ctx context.Context, r *criapi.ImageStatusRequest) (*criapi.ImageStatusResponse, error) {
	log.Tracef("ImageStatus for %q", r.GetImage().GetImage())
	return s.stockImageClient.ImageStatus(ctx, r)
}

// RemoveImage removes the image.
func (s *Service) RemoveImage(ctx context.Context, r *criapi.RemoveImageRequest) (*criapi.RemoveImageResponse, error) {
	log.Debugf("RemoveImage %q", r.GetImage().GetImage())
	return s.stockImageClient.RemoveImage(ctx, r)
}

// ImageFsInfo returns information of the filesystem that is used to store images.
func (s *Service) ImageFsInfo(ctx context.Context, r *criapi.ImageFsInfoRequest) (*criapi.ImageFsInfoResponse, error) {
	log.Debugf("ImageFsInfo")
	return s.stockImageClient.ImageFsInfo(ctx, r)
}

// ContainerStats returns stats of the container. If the container does not
// exist, the call returns an error.
func (s *Service) ContainerStats(ctx context.Context, r *criapi.ContainerStatsRequest) (*criapi.ContainerStatsResponse, error) {
	log.Debugf("ContainerStats for %q", r.GetContainerId())
	return s.stockRuntimeClient.ContainerStats(ctx, r)
}

// ListContainerStats returns stats of all running containers.
func (s *Service) ListContainerStats(ctx context.Context, r *criapi.ListContainerStatsRequest) (*criapi.ListContainerStatsResponse, error) {
	log.Tracef("ListContainerStats with filter %+v", r.GetFilter())
	return s.stockRuntimeClient.ListContainerStats(ctx, r)
}

// Status returns the status of the runtime.
func (s *Service) Status(ctx context.Context, r *criapi.StatusRequest) (*criapi.StatusResponse, error) {
	log.Tracef("Status")
	return s.stockRuntimeClient.Status(ctx, r)
}

// Version returns the runtime name, runtime version, and runtime API version.
func (s *Service) Version(ctx context.Context, r *criapi.VersionRequest) (*criapi.VersionResponse, error) {
	log.Tracef("Version with client side version %q", r.GetVersion())
	return s.stockRuntimeClient.Version(ctx, r)
}

// UpdateRuntimeConfig updates the runtime configuration based on the given request.
func (s *Service) UpdateRuntimeConfig(ctx context.Context, r *criapi.UpdateRuntimeConfigRequest) (*criapi.UpdateRuntimeConfigResponse, error) {
	log.Debugf("UpdateRuntimeConfig with config %+v", r.GetRuntimeConfig())
	return s.stockRuntimeClient.UpdateRuntimeConfig(ctx, r)
}

// ReopenContainerLog asks runtime to reopen the stdout/stderr log file
// for the container.
func (s *Service) ReopenContainerLog(ctx context.Context, r *criapi.ReopenContainerLogRequest) (*criapi.ReopenContainerLogResponse, error) {
	log.Debugf("ReopenContainerLog for %q", r.GetContainerId())
	return s.stockRuntimeClient.ReopenContainerLog(ctx, r)
}
