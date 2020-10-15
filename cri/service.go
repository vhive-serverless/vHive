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
	"path/filepath"

	"github.com/containerd/containerd"
	criconfig "github.com/containerd/cri/pkg/config"
	ctrdcri "github.com/containerd/cri/pkg/server"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	ctrdRoot  = "/var/lib/firecracker-containerd/containerd"
	ctrdState = "/run/firecracker-containerd"
	ctrdSock  = "/run/firecracker-containerd/containerd.sock"
)

type CriService struct {
	criapi.ImageServiceServer
	criapi.RuntimeServiceServer
	ctrdCriService ctrdcri.CRIService
}

func NewCriService(client *containerd.Client) (*CriService, error) {
	config := criconfig.Config{
		PluginConfig:       DefaultConfig(),
		ContainerdRootDir:  filepath.Dir(ctrdRoot),
		ContainerdEndpoint: ctrdSock,
		RootDir:            ctrdRoot,
		StateDir:           ctrdState,
	}

	cs := &CriService{}

	ctrdCriService, err := ctrdcri.NewCRIService(config, client)
	if err != nil {
		return nil, err
	}

	cs.ctrdCriService = ctrdCriService

	return cs, nil
}

func (s *CriService) Register(server *grpc.Server) {
	criapi.RegisterImageServiceServer(server, s)
	criapi.RegisterRuntimeServiceServer(server, s)
}

func (s *CriService) Run() {
	go func() {
		if err := s.ctrdCriService.Run(); err != nil {
			log.WithError(err).Fatal("Failed to run CRI service")
		}
	}()
}

func (s *CriService) Close() error {
	return s.ctrdCriService.Close()
}

func DefaultConfig() criconfig.PluginConfig {
	return criconfig.PluginConfig{
		CniConfig: criconfig.CniConfig{
			NetworkPluginBinDir:       "/opt/cni/bin",
			NetworkPluginConfDir:      "/etc/cni/net.d",
			NetworkPluginMaxConfNum:   1, // only one CNI plugin config file will be loaded
			NetworkPluginConfTemplate: "",
		},
		ContainerdConfig: criconfig.ContainerdConfig{
			Snapshotter:        "devmapper",
			DefaultRuntimeName: "runc",
			NoPivot:            false,
			Runtimes: map[string]criconfig.Runtime{
				"runc": {
					Type: "io.containerd.runc.v1",
				},
			},
		},
		DisableTCPService:   true,
		StreamServerAddress: "127.0.0.1",
		StreamServerPort:    "0",
		StreamIdleTimeout:   "4h",
		EnableSelinux:       false,
		EnableTLSStreaming:  false,
		X509KeyPairStreaming: criconfig.X509KeyPairStreaming{
			TLSKeyFile:  "",
			TLSCertFile: "",
		},
		SandboxImage:            "k8s.gcr.io/pause:3.1",
		StatsCollectPeriod:      10,
		SystemdCgroup:           false,
		MaxContainerLogLineSize: 16 * 1024,
		Registry: criconfig.Registry{
			Mirrors: map[string]criconfig.Mirror{
				"docker.io": {
					Endpoints: []string{"https://registry-1.docker.io"},
				},
			},
		},
		MaxConcurrentDownloads: 3,
		DisableProcMount:       false,
	}
}
