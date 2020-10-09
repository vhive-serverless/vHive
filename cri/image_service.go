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

	"github.com/containerd/containerd"
	containerdimages "github.com/containerd/containerd/images"
	ctrdutil "github.com/containerd/cri/pkg/containerd/util"
	"github.com/containerd/cri/pkg/store"
	imagestore "github.com/containerd/cri/pkg/store/image"
	distribution "github.com/docker/distribution/reference"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	criruntime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

type ImageService struct {
	criruntime.ImageServiceServer
	client      *containerd.Client
	imageStore  *imagestore.Store
	snapshotter string
}

// NewImageService Creates a new image server
func NewImageService(client *containerd.Client, snapshotter string) *ImageService {
	is := &ImageService{
		client:      client,
		imageStore:  imagestore.NewStore(client),
		snapshotter: snapshotter,
	}
	return is
}

// PullImage Pulls an image
func (s *ImageService) PullImage(ctx context.Context, r *criruntime.PullImageRequest) (*criruntime.PullImageResponse, error) {
	log.Debug("Received PullImage")

	ctx = ctrdutil.WithNamespace(ctx)

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

		if err := s.imageStore.Update(ctx, r); err != nil {
			return nil, errors.Wrapf(err, "failed to update image store %q", r)
		}
	}

	log.Debugf("Pulled image %q with image id %q, repo tag %q, repo digest %q", imageRef, imageID,
		repoTag, repoDigest)

	return &criruntime.PullImageResponse{ImageRef: imageID}, nil
}

// RemoveImage Removes an image. Does not return error if image does not exist
func (s *ImageService) RemoveImage(ctx context.Context, r *criruntime.RemoveImageRequest) (*criruntime.RemoveImageResponse, error) {
	// (Plamen) we do not remove the entry for the image from the image store
	_, err := s.localResolve(r.GetImage().GetImage())
	if err != nil {
		if err == store.ErrNotExist {
			// return empty without error when image not found.
			return &criruntime.RemoveImageResponse{}, nil
		}
		return nil, errors.Wrapf(err, "can not resolve %q locally", r.GetImage().GetImage())
	}

	return &criruntime.RemoveImageResponse{}, nil
}

// ListImages Lists the pulled images
func (s *ImageService) ListImages(ctx context.Context, r *criruntime.ListImagesRequest) (*criruntime.ListImagesResponse, error) {
	imagesInStore := s.imageStore.List()

	var images []*criruntime.Image
	for _, image := range imagesInStore {
		images = append(images, toCRIImage(image))
	}

	return &criruntime.ListImagesResponse{Images: images}, nil
}

// ImageFsInfo Returns info about the file system usage. STUB
func (c *ImageService) ImageFsInfo(ctx context.Context, r *criruntime.ImageFsInfoRequest) (*criruntime.ImageFsInfoResponse, error) {
	return &criruntime.ImageFsInfoResponse{
		ImageFilesystems: []*criruntime.FilesystemUsage{
			{
				Timestamp:  int64(1337),
				FsId:       &criruntime.FilesystemIdentifier{Mountpoint: "placeholder"},
				UsedBytes:  &criruntime.UInt64Value{Value: uint64(1337)},
				InodesUsed: &criruntime.UInt64Value{Value: uint64(1337)},
			},
		},
	}, nil
}

// ImageStatus Returns the status of an image
func (s *ImageService) ImageStatus(ctx context.Context, r *criruntime.ImageStatusRequest) (*criruntime.ImageStatusResponse, error) {
	image, err := s.localResolve(r.GetImage().GetImage())
	if err != nil {
		if err == store.ErrNotExist {
			// return empty without error when image not found.
			return &criruntime.ImageStatusResponse{}, nil
		}
		return nil, errors.Wrapf(err, "can not resolve %q locally", r.GetImage().GetImage())
	}

	runtimeImage := toCRIImage(image)
	info, err := s.toCRIImageInfo(ctx, &image, r.GetVerbose())
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate image info")
	}

	return &criruntime.ImageStatusResponse{
		Image: runtimeImage,
		Info:  info,
	}, nil
}
