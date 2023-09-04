// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Amory Hoste and vHive team
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

// Package ctrimages provides an image manager that manages and caches container images.
package image

import (
	"context"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/remotes/docker"
	log "github.com/sirupsen/logrus"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// ImageState is used for synchronization to avoid pulling the same image multiple times concurrently.
type ImageState struct {
	sync.Mutex
	isCached bool
}

// NewImageState creates a new ImageState object that can be used to synchronize pulling a single image
func NewImageState() *ImageState {
	state := new(ImageState)
	state.isCached = false
	return state
}

// ImageManager manages the images that have been pulled to the node.
type ImageManager struct {
	sync.Mutex
	snapshotter  string // image snapshotter
	cachedImages map[string]containerd.Image // Cached container images
	imageStates  map[string]*ImageState
	client       *containerd.Client
}

// NewImageManager creates a new image manager that can be used to fetch container images.
func NewImageManager(client *containerd.Client, snapshotter string) *ImageManager {
	log.Info("Creating image manager")
	manager := new(ImageManager)
	manager.snapshotter = snapshotter
	manager.cachedImages = make(map[string]containerd.Image)
	manager.imageStates = make(map[string]*ImageState)
	manager.client = client
	return manager
}

// pullImage fetches an image and adds it to the cached image list
func (mgr *ImageManager) pullImage(ctx context.Context, imageName string) error {
	var err error
	var image containerd.Image

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
		image, err = mgr.client.Pull(ctx, imageURL,
			containerd.WithPullUnpack,
			containerd.WithPullSnapshotter(mgr.snapshotter),
			containerd.WithResolver(resolver),
		)
	} else {
		// Pull remote image
		image, err = mgr.client.Pull(ctx, imageURL,
			containerd.WithPullUnpack,
			containerd.WithPullSnapshotter(mgr.snapshotter),
		)
	}
	if err != nil {
		return err
	}
	mgr.Lock()
	mgr.cachedImages[imageName] = image
	mgr.Unlock()
	return nil
}

// GetImage fetches an image that can be used to create a container using containerd. Synchronization is implemented
// on a per image level to keep waiting to a minimum.
func (mgr *ImageManager) GetImage(ctx context.Context, imageName string) (*containerd.Image, error) {
	// Get reference to synchronization object for image
	mgr.Lock()
	imgState, found := mgr.imageStates[imageName]
	if !found {
		imgState = NewImageState()
		mgr.imageStates[imageName] = imgState
	}
	mgr.Unlock()

	// Pull image if necessary. The image will only be pulled by the first thread to take the lock.
	imgState.Lock()
	if !imgState.isCached {
		if err := mgr.pullImage(ctx, imageName); err != nil {
			imgState.Unlock()
			return nil, err
		}
		imgState.isCached = true
	}
	imgState.Unlock()

	mgr.Lock()
	image := mgr.cachedImages[imageName]
	mgr.Unlock()

	return &image, nil
}

// Converts an image name to a url if it is not a URL
func getImageURL(image string) string {
	// Pull from dockerhub by default if not specified (default k8s behavior)
	if strings.Contains(image, ".") {
		return image
	}
	return "docker.io/" + image

}

// Checks whether a URL has a .local domain
func isLocalDomain(s string) (bool, error) {
	if ! strings.Contains(s, "://") {
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
