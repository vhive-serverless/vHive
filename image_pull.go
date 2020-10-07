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
	imagedigest "github.com/opencontainers/go-digest"
	log "github.com/sirupsen/logrus"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	containerdimages "github.com/containerd/containerd/images"
	distribution "github.com/docker/distribution/reference"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	criruntime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// PullImage Stub
func (s *imageServer) PullImage(ctx context.Context, r *criruntime.PullImageRequest) (*criruntime.PullImageResponse, error) {
	log.Debug("Received PullImage")

	imageRef := r.GetImage().GetImage()
	namedRef, err := distribution.ParseDockerRef(imageRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse image reference %q", imageRef)
	}

	ref := namedRef.String()
	if ref != imageRef {
		log.Debugf("PullImage using normalized image ref: %q", ref)
	}

	var (
		isSchema1    bool
		imageHandler containerdimages.HandlerFunc = func(_ context.Context,
			desc imagespec.Descriptor) ([]imagespec.Descriptor, error) {
			if desc.MediaType == containerdimages.MediaTypeDockerSchema1Manifest {
				isSchema1 = true
			}
			return nil, nil
		}
	)

	pullOpts := []containerd.RemoteOpt{
		containerd.WithSchema1Conversion,
		containerd.WithPullSnapshotter("devmapper"),
		containerd.WithPullUnpack,
		containerd.WithImageHandler(imageHandler),
	}

	image, err := s.client.Pull(ctx, ref, pullOpts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to pull and unpack image %q", ref)
	}

	configDesc, err := image.Config(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get image config descriptor")
	}

	imageID := configDesc.Digest.String()

	repoDigest, repoTag := getRepoDigestAndTag(namedRef, image.Target().Digest, isSchema1)
	for _, r := range []string{imageID, repoTag, repoDigest} {
		if r == "" {
			continue
		}
		if err := s.createImageReference(ctx, r, image.Target()); err != nil {
			return nil, errors.Wrapf(err, "failed to create image reference %q", r)
		}
		// Update image store to reflect the newest state in containerd.
		// No need to use `updateImage`, because the image reference must
		// have been managed by the cri plugin.
		if err := s.imageStore.Update(ctx, r); err != nil {
			return nil, errors.Wrapf(err, "failed to update image store %q", r)
		}
	}

	log.Debugf("Pulled image %q with image id %q, repo tag %q, repo digest %q", imageRef, imageID,
		repoTag, repoDigest)
	// NOTE(random-liu): the actual state in containerd is the source of truth, even we maintain
	// in-memory image store, it's only for in-memory indexing. The image could be removed
	// by someone else anytime, before/during/after we create the metadata. We should always
	// check the actual state in containerd before using the image or returning status of the
	// image.
	return &criruntime.PullImageResponse{ImageRef: imageID}, nil
}

// getRepoDigestAngTag returns image repoDigest and repoTag of the named image reference.
func getRepoDigestAndTag(namedRef distribution.Named, digest imagedigest.Digest, schema1 bool) (string, string) {
	var repoTag, repoDigest string
	if _, ok := namedRef.(distribution.NamedTagged); ok {
		repoTag = namedRef.String()
	}
	if _, ok := namedRef.(distribution.Canonical); ok {
		repoDigest = namedRef.String()
	} else if !schema1 {
		// digest is not actual repo digest for schema1 image.
		repoDigest = namedRef.Name() + "@" + digest.String()
	}
	return repoDigest, repoTag
}

func (s *imageServer) createImageReference(ctx context.Context, name string, desc imagespec.Descriptor) error {
	img := containerdimages.Image{
		Name:   name,
		Target: desc,
	}
	// TODO(random-liu): Figure out which is the more performant sequence create then update or
	// update then create.
	oldImg, err := s.client.ImageService().Create(ctx, img)
	if err == nil || !errdefs.IsAlreadyExists(err) {
		return err
	}
	if oldImg.Target.Digest == img.Target.Digest {
		return nil
	}
	_, err = s.client.ImageService().Update(ctx, img, "target", "labels")
	return err
}
