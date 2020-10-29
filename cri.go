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

package main

import (
	"context"
	"errors"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/ustiugov/fccd-orchestrator/ctriface"
	"google.golang.org/grpc"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	stockCtrdSockAddr = "/run/containerd/containerd.sock"
	dialTimeout       = 10 * time.Second
	// maxMsgSize use 16MB as the default message size limit.
	// grpc library default is 4MB
	maxMsgSize = 1024 * 1024 * 16
)

type CriService struct {
	criapi.ImageServiceServer
	criapi.RuntimeServiceServer
	orch               *ctriface.Orchestrator
	stockRuntimeClient criapi.RuntimeServiceClient
	stockImageClient   criapi.ImageServiceClient
}

func NewCriService(orch *ctriface.Orchestrator) (*CriService, error) {
	if orch == nil {
		return nil, errors.New("orch must be non nil")
	}

	stockRuntimeClient, err := newStockRuntimeServiceClient()
	if err != nil {
		log.WithError(err).Error("failed to create new stock runtime service client")
		return nil, err
	}

	stockImageClient, err := newStockImageServiceClient()
	if err != nil {
		log.WithError(err).Error("failed to create new stock image service client")
		return nil, err
	}

	cs := &CriService{
		orch:               orch,
		stockRuntimeClient: stockRuntimeClient,
		stockImageClient:   stockImageClient,
	}

	return cs, nil
}

func (s *CriService) Register(server *grpc.Server) {
	criapi.RegisterImageServiceServer(server, s)
	criapi.RegisterRuntimeServiceServer(server, s)
}

func newStockImageServiceClient() (criapi.ImageServiceClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, stockCtrdSockAddr, getDialOpts()...)
	if err != nil {
		return nil, err
	}

	return criapi.NewImageServiceClient(conn), nil
}

func newStockRuntimeServiceClient() (criapi.RuntimeServiceClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, stockCtrdSockAddr, getDialOpts()...)
	if err != nil {
		return nil, err
	}

	return criapi.NewRuntimeServiceClient(conn), nil
}

func dialer(ctx context.Context, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", addr)
}

func getDialOpts() []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithContextDialer(dialer),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize)),
	}
}

func (s *CriService) RunPodSandbox(ctx context.Context, r *criapi.RunPodSandboxRequest) (*criapi.RunPodSandboxResponse, error) {
	//log.Infof("RunPodsandbox for %+v", r.GetConfig().GetMetadata())
	return s.stockRuntimeClient.RunPodSandbox(ctx, r)
}

func (s *CriService) ListPodSandbox(ctx context.Context, r *criapi.ListPodSandboxRequest) (*criapi.ListPodSandboxResponse, error) {
	// log.Tracef("ListPodSandbox with filter %+v", r.GetFilter())
	return s.stockRuntimeClient.ListPodSandbox(ctx, r)
}

func (s *CriService) PodSandboxStatus(ctx context.Context, r *criapi.PodSandboxStatusRequest) (*criapi.PodSandboxStatusResponse, error) {
	//log.Tracef("PodSandboxStatus for %q", r.GetPodSandboxId())
	return s.stockRuntimeClient.PodSandboxStatus(ctx, r)
}

func (s *CriService) StopPodSandbox(ctx context.Context, r *criapi.StopPodSandboxRequest) (*criapi.StopPodSandboxResponse, error) {
	//log.Infof("StopPodSandbox for %q", r.GetPodSandboxId())
	return s.stockRuntimeClient.StopPodSandbox(ctx, r)
}

func (s *CriService) RemovePodSandbox(ctx context.Context, r *criapi.RemovePodSandboxRequest) (*criapi.RemovePodSandboxResponse, error) {
	// log.Infof("RemovePodSandbox for %q", r.GetPodSandboxId())
	return s.stockRuntimeClient.RemovePodSandbox(ctx, r)

}

func (s *CriService) PortForward(ctx context.Context, r *criapi.PortForwardRequest) (*criapi.PortForwardResponse, error) {
	// log.Infof("Portforward for %q port %v", r.GetPodSandboxId(), r.GetPort())
	return s.stockRuntimeClient.PortForward(ctx, r)
}

func (s *CriService) StartContainer(ctx context.Context, r *criapi.StartContainerRequest) (*criapi.StartContainerResponse, error) {
	// log.Infof("StartContainer for %q", r.GetContainerId())
	return s.stockRuntimeClient.StartContainer(ctx, r)

}

func (s *CriService) ListContainers(ctx context.Context, r *criapi.ListContainersRequest) (*criapi.ListContainersResponse, error) {
	// log.Tracef("ListContainers with filter %+v", r.GetFilter())
	return s.stockRuntimeClient.ListContainers(ctx, r)
}

func (s *CriService) ContainerStatus(ctx context.Context, r *criapi.ContainerStatusRequest) (*criapi.ContainerStatusResponse, error) {
	// log.Tracef("ContainerStatus for %q", r.GetContainerId())
	return s.stockRuntimeClient.ContainerStatus(ctx, r)
}

func (s *CriService) StopContainer(ctx context.Context, r *criapi.StopContainerRequest) (*criapi.StopContainerResponse, error) {
	// log.Infof("StopContainer for %q with timeout %d (s)", r.GetContainerId(), r.GetTimeout())
	return s.stockRuntimeClient.StopContainer(ctx, r)
}

func (s *CriService) RemoveContainer(ctx context.Context, r *criapi.RemoveContainerRequest) (*criapi.RemoveContainerResponse, error) {
	// log.Infof("RemoveContainer for %q", r.GetContainerId())
	return s.stockRuntimeClient.RemoveContainer(ctx, r)
}

func (s *CriService) ExecSync(ctx context.Context, r *criapi.ExecSyncRequest) (*criapi.ExecSyncResponse, error) {
	// log.Infof("ExecSync for %q with command %+v and timeout %d (s)", r.GetContainerId(), r.GetCmd(), r.GetTimeout())
	return s.stockRuntimeClient.ExecSync(ctx, r)
}

func (s *CriService) Exec(ctx context.Context, r *criapi.ExecRequest) (*criapi.ExecResponse, error) {
	// log.Infof("Exec for %q with command %+v, tty %v and stdin %v",
	return s.stockRuntimeClient.Exec(ctx, r)
}

func (s *CriService) Attach(ctx context.Context, r *criapi.AttachRequest) (*criapi.AttachResponse, error) {
	//log.Infof("Attach for %q with tty %v and stdin %v", r.GetContainerId(), r.GetTty(), r.GetStdin())
	return s.stockRuntimeClient.Attach(ctx, r)
}

func (s *CriService) UpdateContainerResources(ctx context.Context, r *criapi.UpdateContainerResourcesRequest) (*criapi.UpdateContainerResourcesResponse, error) {
	// log.Infof("UpdateContainerResources for %q with %+v", r.GetContainerId(), r.GetLinux())
	return s.stockRuntimeClient.UpdateContainerResources(ctx, r)
}

func (s *CriService) PullImage(ctx context.Context, r *criapi.PullImageRequest) (*criapi.PullImageResponse, error) {
	// log.Infof("PullImage %q", r.GetImage().GetImage())
	return s.stockImageClient.PullImage(ctx, r)
}

func (s *CriService) ListImages(ctx context.Context, r *criapi.ListImagesRequest) (*criapi.ListImagesResponse, error) {
	// log.Tracef("ListImages with filter %+v", r.GetFilter())
	return s.stockImageClient.ListImages(ctx, r)
}

func (s *CriService) ImageStatus(ctx context.Context, r *criapi.ImageStatusRequest) (*criapi.ImageStatusResponse, error) {
	// log.Tracef("ImageStatus for %q", r.GetImage().GetImage())
	return s.stockImageClient.ImageStatus(ctx, r)
}

func (s *CriService) RemoveImage(ctx context.Context, r *criapi.RemoveImageRequest) (*criapi.RemoveImageResponse, error) {
	// log.Infof("RemoveImage %q", r.GetImage().GetImage())
	return s.stockImageClient.RemoveImage(ctx, r)
}

func (s *CriService) ImageFsInfo(ctx context.Context, r *criapi.ImageFsInfoRequest) (*criapi.ImageFsInfoResponse, error) {
	// log.Debugf("ImageFsInfo")
	return s.stockImageClient.ImageFsInfo(ctx, r)
}

func (s *CriService) ContainerStats(ctx context.Context, r *criapi.ContainerStatsRequest) (*criapi.ContainerStatsResponse, error) {
	// log.Debugf("ContainerStats for %q", r.GetContainerId())
	return s.stockRuntimeClient.ContainerStats(ctx, r)
}

func (s *CriService) ListContainerStats(ctx context.Context, r *criapi.ListContainerStatsRequest) (*criapi.ListContainerStatsResponse, error) {
	// log.Tracef("ListContainerStats with filter %+v", r.GetFilter())
	return s.stockRuntimeClient.ListContainerStats(ctx, r)
}

func (s *CriService) Status(ctx context.Context, r *criapi.StatusRequest) (*criapi.StatusResponse, error) {
	// log.Tracef("Status")
	return s.stockRuntimeClient.Status(ctx, r)
}

func (s *CriService) Version(ctx context.Context, r *criapi.VersionRequest) (*criapi.VersionResponse, error) {
	// log.Tracef("Version with client side version %q", r.GetVersion())
	return s.stockRuntimeClient.Version(ctx, r)
}

func (s *CriService) UpdateRuntimeConfig(ctx context.Context, r *criapi.UpdateRuntimeConfigRequest) (*criapi.UpdateRuntimeConfigResponse, error) {
	// log..Debugf("UpdateRuntimeConfig with config %+v", r.GetRuntimeConfig())
	return s.stockRuntimeClient.UpdateRuntimeConfig(ctx, r)
}

func (s *CriService) ReopenContainerLog(ctx context.Context, r *criapi.ReopenContainerLogRequest) (*criapi.ReopenContainerLogResponse, error) {
	// log.Debugf("ReopenContainerLog for %q", r.GetContainerId())
	return s.stockRuntimeClient.ReopenContainerLog(ctx, r)
}
