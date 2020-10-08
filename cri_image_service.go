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
	"encoding/json"
	"strconv"
	"strings"

	imagedigest "github.com/opencontainers/go-digest"
	log "github.com/sirupsen/logrus"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	containerdimages "github.com/containerd/containerd/images"
	ctrdutil "github.com/containerd/cri/pkg/containerd/util"
	"github.com/containerd/cri/pkg/store"
	imagestore "github.com/containerd/cri/pkg/store/image"
	"github.com/docker/distribution/reference"
	distribution "github.com/docker/distribution/reference"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	criruntime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

type imageServer struct {
	criruntime.ImageServiceServer
	client     *containerd.Client
	imageStore *imagestore.Store
}

// NewImageServer Creates a new image server
func NewImageServer(client *containerd.Client) *imageServer {
	is := &imageServer{
		client:     client,
		imageStore: imagestore.NewStore(client),
	}

	return is
}

// PullImage Pulls an image
func (s *imageServer) PullImage(ctx context.Context, r *criruntime.PullImageRequest) (*criruntime.PullImageResponse, error) {
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
func (s *imageServer) RemoveImage(ctx context.Context, r *criruntime.RemoveImageRequest) (*criruntime.RemoveImageResponse, error) {
	// (Plamen) Does not implement the full remove image functionality. we do not remove the entry for the image from the image store
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
func (s *imageServer) ListImages(ctx context.Context, r *criruntime.ListImagesRequest) (*criruntime.ListImagesResponse, error) {
	imagesInStore := s.imageStore.List()

	var images []*criruntime.Image
	for _, image := range imagesInStore {
		images = append(images, toCRIImage(image))
	}

	return &criruntime.ListImagesResponse{Images: images}, nil
}

// ImageFsInfo Returns info about the file system usage. STUB
func (c *imageServer) ImageFsInfo(ctx context.Context, r *criruntime.ImageFsInfoRequest) (*criruntime.ImageFsInfoResponse, error) {
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
func (s *imageServer) ImageStatus(ctx context.Context, r *criruntime.ImageStatusRequest) (*criruntime.ImageStatusResponse, error) {
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

// returns the image associated with the ref
func (s *imageServer) localResolve(refOrID string) (imagestore.Image, error) {
	getImageID := func(refOrId string) string {
		if _, err := imagedigest.Parse(refOrID); err == nil {
			return refOrID
		}
		return func(ref string) string {
			// ref is not image id, try to resolve it locally.
			// TODO(random-liu): Handle this error better for debugging.
			normalized, err := reference.ParseDockerRef(ref)
			if err != nil {
				return ""
			}
			id, err := s.imageStore.Resolve(normalized.String())
			if err != nil {
				return ""
			}
			return id
		}(refOrID)
	}

	imageID := getImageID(refOrID)
	if imageID == "" {
		// Try to treat ref as imageID
		imageID = refOrID
	}
	return s.imageStore.Get(imageID)
}

type verboseImageInfo struct {
	ChainID   string          `json:"chainID"`
	ImageSpec imagespec.Image `json:"imageSpec"`
}

// toCRIImage converts internal image object to CRI runtime.Image.
func toCRIImage(image imagestore.Image) *criruntime.Image {
	repoTags, repoDigests := parseImageReferences(image.References)
	runtimeImage := &criruntime.Image{
		Id:          image.ID,
		RepoTags:    repoTags,
		RepoDigests: repoDigests,
		Size_:       uint64(image.Size),
	}
	uid, username := getUserFromImage(image.ImageSpec.Config.User)
	if uid != nil {
		runtimeImage.Uid = &criruntime.Int64Value{Value: *uid}
	}
	runtimeImage.Username = username

	return runtimeImage
}

// toCRIImageInfo converts internal image object information to CRI image status response info map.
func (s *imageServer) toCRIImageInfo(ctx context.Context, image *imagestore.Image, verbose bool) (map[string]string, error) {
	if !verbose {
		return nil, nil
	}

	info := make(map[string]string)

	imi := &verboseImageInfo{
		ChainID:   image.ChainID,
		ImageSpec: image.ImageSpec,
	}

	m, err := json.Marshal(imi)
	if err == nil {
		info["info"] = string(m)
	} else {
		log.WithError(err).Errorf("failed to marshal info %v", imi)
		info["info"] = err.Error()
	}

	return info, nil
}

func parseImageReferences(refs []string) ([]string, []string) {
	var tags, digests []string
	for _, ref := range refs {
		parsed, err := reference.ParseAnyReference(ref)
		if err != nil {
			continue
		}
		if _, ok := parsed.(reference.Canonical); ok {
			digests = append(digests, parsed.String())
		} else if _, ok := parsed.(reference.Tagged); ok {
			tags = append(tags, parsed.String())
		}
	}
	return tags, digests
}

func getUserFromImage(user string) (*int64, string) {
	// return both empty if user is not specified in the image.
	if user == "" {
		return nil, ""
	}
	// split instances where the id may contain user:group
	user = strings.Split(user, ":")[0]
	// user could be either uid or user name. Try to interpret as numeric uid.
	uid, err := strconv.ParseInt(user, 10, 64)
	if err != nil {
		// If user is non numeric, assume it's user name.
		return nil, user
	}
	// If user is a numeric uid.
	return &uid, ""
}
