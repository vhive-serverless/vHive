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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	criruntime "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func TestImageFsInfo(t *testing.T) {
	is := &ImageService{}

	expected := &criruntime.FilesystemUsage{
		Timestamp:  1337,
		FsId:       &criruntime.FilesystemIdentifier{Mountpoint: "placeholder"},
		UsedBytes:  &criruntime.UInt64Value{Value: 1337},
		InodesUsed: &criruntime.UInt64Value{Value: 1337},
	}

	resp, err := is.ImageFsInfo(context.Background(), &criruntime.ImageFsInfoRequest{})
	require.NoError(t, err)
	stats := resp.GetImageFilesystems()
	assert.Len(t, stats, 1)
	assert.Equal(t, expected, stats[0])
}
