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

	"github.com/ustiugov/fccd-orchestrator/ctriface"

	log "github.com/sirupsen/logrus"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	userContainerName = "user-container"
	guestIPEnv        = "GUESTIP"
	guestPortEnv      = "GUESTPORT"
)

func (s *CriService) CreateContainer(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	// log.Infof("CreateContainer within sandbox %q for container %+v",
	// 	r.GetPodSandboxId(), r.GetConfig().GetMetadata())
	var (
		resp *ctriface.StartVMResponse
		err  error
	)

	config := r.GetConfig()

	containerName := config.GetMetadata().GetName()
	log.Infof("received CreateContainer for %s.", containerName)
	if containerName == userContainerName {
		// TODO: Start VM, get guestIP and pass to CreateContainerRequest
		log.Info("UC request")

		resp, _, err = s.orch.StartVM(context.Background(), "1", "crccheck/hello-world:latest")
		if err != nil {
			log.WithError(err).Error("failed to start VM")
			return nil, err
		}
	}

	envs := config.GetEnvs()
	guestIPEnv := &criapi.KeyValue{Key: guestIPEnv, Value: resp.GuestIP}
	guestPortEnv := &criapi.KeyValue{Key: guestPortEnv, Value: "8000"}
	envs = append(envs, guestIPEnv, guestPortEnv)
	r.Config.Envs = envs

	return s.stockRuntimeClient.CreateContainer(ctx, r)
}
