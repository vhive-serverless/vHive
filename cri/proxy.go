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

	ctrdutil "github.com/containerd/cri/pkg/containerd/util"
	log "github.com/sirupsen/logrus"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// TODO: Check initialized

func (s *CriService) RunPodSandbox(ctx context.Context, r *criapi.RunPodSandboxRequest) (*criapi.RunPodSandboxResponse, error) {
	//log.Infof("RunPodsandbox for %+v", r.GetConfig().GetMetadata())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("RunPodSandbox for %+v failed, error", r.GetConfig().GetMetadata())
	// 	} else {
	// 		log.Infof("RunPodSandbox for %+v returns sandbox id %q", r.GetConfig().GetMetadata(), res.GetPodSandboxId())
	// 	}
	// }()
	return s.ctrdCriService.RunPodSandbox(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) ListPodSandbox(ctx context.Context, r *criapi.ListPodSandboxRequest) (*criapi.ListPodSandboxResponse, error) {
	// log.Tracef("ListPodSandbox with filter %+v", r.GetFilter())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Error("ListPodSandbox failed")
	// 	} else {
	// 		log.Tracef("ListPodSandbox returns pod sandboxes %+v", res.GetItems())
	// 	}
	// }()
	return s.ctrdCriService.ListPodSandbox(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) PodSandboxStatus(ctx context.Context, r *criapi.PodSandboxStatusRequest) (*criapi.PodSandboxStatusResponse, error) {
	//log.Tracef("PodSandboxStatus for %q", r.GetPodSandboxId())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("PodSandboxStatus for %q failed", r.GetPodSandboxId())
	// 	} else {
	// 		log.Tracef("PodSandboxStatus for %q returns status %+v", r.GetPodSandboxId(), res.GetStatus())
	// 	}
	// }()
	return s.ctrdCriService.PodSandboxStatus(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) StopPodSandbox(ctx context.Context, r *criapi.StopPodSandboxRequest) (*criapi.StopPodSandboxResponse, error) {
	//log.Infof("StopPodSandbox for %q", r.GetPodSandboxId())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("StopPodSandbox for %q failed", r.GetPodSandboxId())
	// 	} else {
	// 		log.Infof("StopPodSandbox for %q returns successfully", r.GetPodSandboxId())
	// 	}
	// }()
	return s.ctrdCriService.StopPodSandbox(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) RemovePodSandbox(ctx context.Context, r *criapi.RemovePodSandboxRequest) (*criapi.RemovePodSandboxResponse, error) {
	// log.Infof("RemovePodSandbox for %q", r.GetPodSandboxId())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("RemovePodSandbox for %q failed", r.GetPodSandboxId())
	// 	} else {
	// 		log.Infof("RemovePodSandbox %q returns successfully", r.GetPodSandboxId())
	// 	}
	// }()
	return s.ctrdCriService.RemovePodSandbox(ctrdutil.WithNamespace(ctx), r)

}

func (s *CriService) PortForward(ctx context.Context, r *criapi.PortForwardRequest) (*criapi.PortForwardResponse, error) {
	// log.Infof("Portforward for %q port %v", r.GetPodSandboxId(), r.GetPort())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("Portforward for %q failed", r.GetPodSandboxId())
	// 	} else {
	// 		log.Infof("Portforward for %q returns URL %q", r.GetPodSandboxId(), res.GetUrl())
	// 	}
	// }()
	return s.ctrdCriService.PortForward(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) CreateContainer(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	// log.Infof("CreateContainer within sandbox %q for container %+v",
	// 	r.GetPodSandboxId(), r.GetConfig().GetMetadata())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("CreateContainer within sandbox %q for %+v failed",
	// 			r.GetPodSandboxId(), r.GetConfig().GetMetadata())
	// 	} else {
	// 		log.Infof("CreateContainer within sandbox %q for %+v returns container id %q",
	// 			r.GetPodSandboxId(), r.GetConfig().GetMetadata(), res.GetContainerId())
	// 	}
	// }()
	return s.ctrdCriService.CreateContainer(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) StartContainer(ctx context.Context, r *criapi.StartContainerRequest) (*criapi.StartContainerResponse, error) {
	// log.Infof("StartContainer for %q", r.GetContainerId())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("StartContainer for %q failed", r.GetContainerId())
	// 	} else {
	// 		log.Infof("StartContainer for %q returns successfully", r.GetContainerId())
	// 	}
	// }()
	return s.ctrdCriService.StartContainer(ctrdutil.WithNamespace(ctx), r)

}

func (s *CriService) ListContainers(ctx context.Context, r *criapi.ListContainersRequest) (*criapi.ListContainersResponse, error) {
	// log.Tracef("ListContainers with filter %+v", r.GetFilter())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("ListContainers with filter %+v failed", r.GetFilter())
	// 	} else {
	// 		log.Tracef("ListContainers with filter %+v returns containers %+v",
	// 			r.GetFilter(), res.GetContainers())
	// 	}
	// }()
	return s.ctrdCriService.ListContainers(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) ContainerStatus(ctx context.Context, r *criapi.ContainerStatusRequest) (*criapi.ContainerStatusResponse, error) {
	// log.Tracef("ContainerStatus for %q", r.GetContainerId())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("ContainerStatus for %q failed", r.GetContainerId())
	// 	} else {
	// 		log.Tracef("ContainerStatus for %q returns status %+v", r.GetContainerId(), res.GetStatus())
	// 	}
	// }()
	return s.ctrdCriService.ContainerStatus(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) StopContainer(ctx context.Context, r *criapi.StopContainerRequest) (*criapi.StopContainerResponse, error) {
	// log.Infof("StopContainer for %q with timeout %d (s)", r.GetContainerId(), r.GetTimeout())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("StopContainer for %q failed", r.GetContainerId())
	// 	} else {
	// 		log.Infof("StopContainer for %q returns successfully", r.GetContainerId())
	// 	}
	// }()
	return s.ctrdCriService.StopContainer(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) RemoveContainer(ctx context.Context, r *criapi.RemoveContainerRequest) (*criapi.RemoveContainerResponse, error) {
	// log.Infof("RemoveContainer for %q", r.GetContainerId())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("RemoveContainer for %q failed", r.GetContainerId())
	// 	} else {
	// 		log.Infof("RemoveContainer for %q returns successfully", r.GetContainerId())
	// 	}
	// }()
	return s.ctrdCriService.RemoveContainer(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) ExecSync(ctx context.Context, r *criapi.ExecSyncRequest) (*criapi.ExecSyncResponse, error) {
	// log.Infof("ExecSync for %q with command %+v and timeout %d (s)", r.GetContainerId(), r.GetCmd(), r.GetTimeout())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("ExecSync for %q failed", r.GetContainerId())
	// 	} else {
	// 		log.Infof("ExecSync for %q returns with exit code %d", r.GetContainerId(), res.GetExitCode())
	// 		log.Debugf("ExecSync for %q outputs - stdout: %q, stderr: %q", r.GetContainerId(),
	// 			res.GetStdout(), res.GetStderr())
	// 	}
	// }()
	return s.ctrdCriService.ExecSync(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) Exec(ctx context.Context, r *criapi.ExecRequest) (*criapi.ExecResponse, error) {
	// log.Infof("Exec for %q with command %+v, tty %v and stdin %v",
	// 	r.GetContainerId(), r.GetCmd(), r.GetTty(), r.GetStdin())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("Exec for %q failed", r.GetContainerId())
	// 	} else {
	// 		log.Infof("Exec for %q returns URL %q", r.GetContainerId(), res.GetUrl())
	// 	}
	// }()
	return s.ctrdCriService.Exec(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) Attach(ctx context.Context, r *criapi.AttachRequest) (*criapi.AttachResponse, error) {
	log.Infof("Attach for %q with tty %v and stdin %v", r.GetContainerId(), r.GetTty(), r.GetStdin())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("Attach for %q failed", r.GetContainerId())
	// 	} else {
	// 		log.Infof("Attach for %q returns URL %q", r.GetContainerId(), res.Url)
	// 	}
	// }()
	return s.ctrdCriService.Attach(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) UpdateContainerResources(ctx context.Context, r *criapi.UpdateContainerResourcesRequest) (*criapi.UpdateContainerResourcesResponse, error) {
	// log.Infof("UpdateContainerResources for %q with %+v", r.GetContainerId(), r.GetLinux())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("UpdateContainerResources for %q failed", r.GetContainerId())
	// 	} else {
	// 		log.Infof("UpdateContainerResources for %q returns successfully", r.GetContainerId())
	// 	}
	// }()
	return s.ctrdCriService.UpdateContainerResources(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) PullImage(ctx context.Context, r *criapi.PullImageRequest) (*criapi.PullImageResponse, error) {
	// log.Infof("PullImage %q", r.GetImage().GetImage())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("PullImage %q failed", r.GetImage().GetImage())
	// 	} else {
	// 		log.Infof("PullImage %q returns image reference %q",
	// 			r.GetImage().GetImage(), res.GetImageRef())
	// 	}
	// }()
	return s.ctrdCriService.PullImage(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) ListImages(ctx context.Context, r *criapi.ListImagesRequest) (*criapi.ListImagesResponse, error) {
	// log.Tracef("ListImages with filter %+v", r.GetFilter())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("ListImages with filter %+v failed", r.GetFilter())
	// 	} else {
	// 		log.Tracef("ListImages with filter %+v returns image list %+v",
	// 			r.GetFilter(), res.GetImages())
	// 	}
	// }()
	return s.ctrdCriService.ListImages(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) ImageStatus(ctx context.Context, r *criapi.ImageStatusRequest) (*criapi.ImageStatusResponse, error) {
	// log.Tracef("ImageStatus for %q", r.GetImage().GetImage())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("ImageStatus for %q failed", r.GetImage().GetImage())
	// 	} else {
	// 		log.Tracef("ImageStatus for %q returns image status %+v",
	// 			r.GetImage().GetImage(), res.GetImage())
	// 	}
	// }()
	return s.ctrdCriService.ImageStatus(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) RemoveImage(ctx context.Context, r *criapi.RemoveImageRequest) (*criapi.RemoveImageResponse, error) {
	// log.Infof("RemoveImage %q", r.GetImage().GetImage())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("RemoveImage %q failed", r.GetImage().GetImage())
	// 	} else {
	// 		log.Infof("RemoveImage %q returns successfully", r.GetImage().GetImage())
	// 	}
	// }()
	return s.ctrdCriService.RemoveImage(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) ImageFsInfo(ctx context.Context, r *criapi.ImageFsInfoRequest) (*criapi.ImageFsInfoResponse, error) {
	// log.Debugf("ImageFsInfo")
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Error("ImageFsInfo failed")
	// 	} else {
	// 		log.Debugf("ImageFsInfo returns filesystem info %+v", res.ImageFilesystems)
	// 	}
	// }()
	return s.ctrdCriService.ImageFsInfo(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) ContainerStats(ctx context.Context, r *criapi.ContainerStatsRequest) (*criapi.ContainerStatsResponse, error) {
	// log.Debugf("ContainerStats for %q", r.GetContainerId())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("ContainerStats for %q failed", r.GetContainerId())
	// 	} else {
	// 		log.Debugf("ContainerStats for %q returns stats %+v", r.GetContainerId(), res.GetStats())
	// 	}
	// }()
	return s.ctrdCriService.ContainerStats(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) ListContainerStats(ctx context.Context, r *criapi.ListContainerStatsRequest) (*criapi.ListContainerStatsResponse, error) {
	// log.Tracef("ListContainerStats with filter %+v", r.GetFilter())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Error("ListContainerStats failed")
	// 	} else {
	// 		log.Tracef("ListContainerStats returns stats %+v", res.GetStats())
	// 	}
	// }()
	return s.ctrdCriService.ListContainerStats(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) Status(ctx context.Context, r *criapi.StatusRequest) (*criapi.StatusResponse, error) {
	// log.Tracef("Status")
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Error("Status failed")
	// 	} else {
	// 		log.Tracef("Status returns status %+v", res.GetStatus())
	// 	}
	// }()
	return s.ctrdCriService.Status(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) Version(ctx context.Context, r *criapi.VersionRequest) (*criapi.VersionResponse, error) {
	// log.Tracef("Version with client side version %q", r.GetVersion())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Error("Version failed")
	// 	} else {
	// 		log.Tracef("Version returns %+v", res)
	// 	}
	// }()
	return s.ctrdCriService.Version(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) UpdateRuntimeConfig(ctx context.Context, r *criapi.UpdateRuntimeConfigRequest) (*criapi.UpdateRuntimeConfigResponse, error) {
	// log..Debugf("UpdateRuntimeConfig with config %+v", r.GetRuntimeConfig())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Error("UpdateRuntimeConfig failed")
	// 	} else {
	// 		log.Debug("UpdateRuntimeConfig returns returns successfully")
	// 	}
	// }()
	return s.ctrdCriService.UpdateRuntimeConfig(ctrdutil.WithNamespace(ctx), r)
}

func (s *CriService) ReopenContainerLog(ctx context.Context, r *criapi.ReopenContainerLogRequest) (*criapi.ReopenContainerLogResponse, error) {
	// log.Debugf("ReopenContainerLog for %q", r.GetContainerId())
	// defer func() {
	// 	if err != nil {
	// 		log.WithError(err).Errorf("ReopenContainerLog for %q failed", r.GetContainerId())
	// 	} else {
	// 		log.Debugf("ReopenContainerLog for %q returns successfully", r.GetContainerId())
	// 	}
	// }()
	return s.ctrdCriService.ReopenContainerLog(ctrdutil.WithNamespace(ctx), r)
}
