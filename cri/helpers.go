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
	"encoding/json"
	"strconv"
	"strings"

	"github.com/containerd/containerd/errdefs"
	containerdimages "github.com/containerd/containerd/images"
	imagestore "github.com/containerd/cri/pkg/store/image"
	"github.com/docker/distribution/reference"
	distribution "github.com/docker/distribution/reference"
	imagedigest "github.com/opencontainers/go-digest"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	log "github.com/sirupsen/logrus"
	criruntime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

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

// returns the image associated with the ref
func (s *ImageService) localResolve(refOrID string) (imagestore.Image, error) {
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
func (s *ImageService) toCRIImageInfo(ctx context.Context, image *imagestore.Image, verbose bool) (map[string]string, error) {
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

func (s *ImageService) createImageReference(ctx context.Context, name string, desc imagespec.Descriptor) error {
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
