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
	"testing"

	imagestore "github.com/containerd/cri/pkg/store/image"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	criruntime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func TestImageStatus(t *testing.T) {
	is := &ImageService{imageStore: imagestore.NewStore(nil)}

	testID := "sha256:d848ce12891bf78792cda4a23c58984033b0c397a55e93a1556202222ecc5ed4"

	image := imagestore.Image{
		ID:      testID,
		ChainID: "test-chain-id",
		References: []string{
			"gcr.io/library/busybox:latest",
			"gcr.io/library/busybox@sha256:e6693c20186f837fc393390135d8a598a96a833917917789d63766cab6c59582",
		},
		Size: 1234,
		ImageSpec: imagespec.Image{
			Config: imagespec.ImageConfig{
				User: "user:group",
			},
		},
	}
	expected := &criruntime.Image{
		Id:          testID,
		RepoTags:    []string{"gcr.io/library/busybox:latest"},
		RepoDigests: []string{"gcr.io/library/busybox@sha256:e6693c20186f837fc393390135d8a598a96a833917917789d63766cab6c59582"},
		Size_:       uint64(1234),
		Username:    "user",
	}

	resp, err := is.ImageStatus(context.Background(), &criruntime.ImageStatusRequest{
		Image: &criruntime.ImageSpec{Image: testID},
	})
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Nil(t, resp.GetImage())

	is.imageStore, err = imagestore.NewFakeStore([]imagestore.Image{image})
	assert.NoError(t, err)

	resp, err = is.ImageStatus(context.Background(), &criruntime.ImageStatusRequest{
		Image: &criruntime.ImageSpec{Image: testID},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, expected, resp.GetImage())
}
