// MIT License
//
// Copyright (c) 2020 Nathaniel Tornow and EASE lab
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

package gvisor

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/containerd/go-cni"
	log "github.com/sirupsen/logrus"
)

const (
	gvisorContainerdAddress = "/run/gvisor-containerd/gvisor-containerd.sock"
	gvisorRuntime           = "io.containerd.runsc.v1"
	namespaceName           = "default"
	bridgeConfFile          = "/etc/cni/net.d/10-bridge.conf"
)

type coordinator struct {
	sync.Mutex
	nextID           uint64
	client           *containerd.Client
	activeContainers map[string]*gvContainer
	network          cni.CNI
	cachedImages     map[string]containerd.Image
}

type gvContainer struct {
	container containerd.Container
	task      containerd.Task
	taskC     <-chan containerd.ExitStatus
	ip        string
}

func newCoordinator() (*coordinator, error) {
	client, err := containerd.New(gvisorContainerdAddress, containerd.WithDefaultRuntime(gvisorRuntime))
	if err != nil {
		return nil, fmt.Errorf("failed to start containerd client: %v", err)
	}
	c := new(coordinator)
	c.client = client
	c.activeContainers = make(map[string]*gvContainer)
	network, err := cni.New(cni.WithConfFile(bridgeConfFile))
	if err != nil {
		return nil, fmt.Errorf("failed to init cni: %v", err)
	}
	c.network = network
	c.cachedImages = make(map[string]containerd.Image)
	return c, nil
}

func (c *coordinator) startContainer(ctx context.Context, imageName string) (_ *gvContainer, retErr error) {
	ctx = namespaces.WithNamespace(ctx, namespaceName)
	ctrID := strconv.Itoa(int(atomic.AddUint64(&c.nextID, 1)))
	image, err := c.getImage(ctx, imageName)
	if err != nil {
		return nil, err
	}
	container, err := c.client.NewContainer(
		ctx,
		ctrID,
		containerd.WithImage(*image),
		containerd.WithNewSnapshot(ctrID, *image),
		containerd.WithNewSpec(oci.WithImageConfig(*image)),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			if err := container.Delete(ctx); err != nil {
				log.Errorf("failed to delete container after failure: %v", retErr)
			}
		}
	}()

	// create a task from the container
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			if err := task.Kill(ctx, syscall.SIGTERM); err != nil {
				log.Errorf("failed to kill task after failure: %v", retErr)
			}
		}
	}()

	exitStatusC, err := task.Wait(ctx)
	if err != nil {
		return nil, err
	}

	netns := fmt.Sprintf("/proc/%v/ns/net", task.Pid())
	result, err := c.network.Setup(ctx, ctrID, netns)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	defer func() {
		if retErr != nil {
			if err := c.network.Remove(ctx, ctrID, netns); err != nil {
				log.Errorf("failed to remove network after failure: %v", retErr)
			}
		}
	}()

	ip := result.Interfaces["eth0"].IPConfigs[0].IP.String()

	if err := task.Start(ctx); err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			if _, err := task.Delete(ctx); err != nil {
				log.Errorf("failed to delete task after failure: %v", retErr)
			}
		}
	}()

	return &gvContainer{ip: ip, container: container, task: task, taskC: exitStatusC}, nil
}

func (c *coordinator) stopContainer(ctx context.Context, containerID string) error {
	ctx = namespaces.WithNamespace(ctx, namespaceName)
	c.Lock()
	ctr, ok := c.activeContainers[containerID]
	c.Unlock()
	if !ok {
		return fmt.Errorf("failed to find a active container with id %v", containerID)
	}
	container := ctr.container
	task := ctr.task

	netns := fmt.Sprintf("/proc/%v/ns/net", task.Pid())
	err := c.network.Remove(ctx, containerID, netns)
	if err != nil {
		log.Errorf("failed to teardown network: %v", err)
	}

	err = task.Kill(ctx, syscall.SIGTERM)
	if err != nil {
		return fmt.Errorf("failed to kill task: %v", err)
	}
	<-ctr.taskC
	_, err = task.Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete task: %v", err)
	}
	err = container.Delete(ctx, containerd.WithSnapshotCleanup)
	if err != nil {
		return fmt.Errorf("failed to delete container: %v", err)
	}
	return nil
}

func (c *coordinator) insertActive(containerID string, ctr *gvContainer) {
	c.Lock()
	c.activeContainers[containerID] = ctr
	c.Unlock()
}

// Checks whether a URL has a .local domain
func isLocalDomain(s string) (bool, error) {
	if !strings.Contains(s, "://") {
		s = "dummy://" + s
	}

	u, err := url.Parse(s)
	if err != nil {
		return false, err
	}

	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
	}

	i := strings.LastIndex(host, ".")
	tld := host[i+1:]

	return tld == "local", nil
}

// Converts an image name to a url if it is not a URL
func getImageURL(image string) string {
	// Pull from dockerhub by default if not specified (default k8s behavior)
	if strings.Contains(image, ".") {
		return image
	}
	return "docker.io/" + image

}

func (c *coordinator) getImage(ctx context.Context, imageName string) (*containerd.Image, error) {
	image, found := c.cachedImages[imageName]
	if !found {
		var err error
		log.Debug(fmt.Sprintf("Pulling image %s", imageName))

		imageURL := getImageURL(imageName)
		local, _ := isLocalDomain(imageURL)
		if local {
			// Pull local image using HTTP
			resolver := docker.NewResolver(docker.ResolverOptions{
				Client: http.DefaultClient,
				Hosts: docker.ConfigureDefaultRegistries(
					docker.WithPlainHTTP(docker.MatchAllHosts),
				),
			})
			image, err = c.client.Pull(ctx, imageURL,
				containerd.WithPullUnpack,
				containerd.WithResolver(resolver),
			)
		} else {
			// Pull remote image
			image, err = c.client.Pull(ctx, imageURL,
				containerd.WithPullUnpack,
			)
		}

		if err != nil {
			return &image, err
		}
		c.cachedImages[imageName] = image
	}

	return &image, nil
}
