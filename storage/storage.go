// MIT License
//
// Copyright (c) 2025 André Jesus and vHive team
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

package storage

import (
	"context"
	"io"
)

// RemoteFetchClassStats reports successful logical GET operations and the
// response bytes delivered to the caller. It deliberately excludes HEAD,
// LIST, uploads, and bytes retried internally by the HTTP transport.
type RemoteFetchClassStats struct {
	Requests uint64 `json:"requests"`
	Bytes    uint64 `json:"bytes"`
}

// RemoteFetchStats is a process-local snapshot of object-store reads. Classes
// are stable strings so evaluation runners can validate individual data paths
// without parsing a high-volume external MinIO trace.
type RemoteFetchStats struct {
	// A strict local manager has no object store, so no remote read can occur.
	StorageDisabled bool                             `json:"storage_disabled,omitempty"`
	Total           RemoteFetchClassStats            `json:"total"`
	Classes         map[string]RemoteFetchClassStats `json:"classes"`
}

// RemoteFetchStatsStorage is implemented by object stores that expose exact
// process-local response-byte accounting. Reset must only be used at a quiescent
// experiment boundary.
type RemoteFetchStatsStorage interface {
	SnapshotRemoteFetchStats() RemoteFetchStats
	ResetRemoteFetchStats()
}

// ObjectStorage defines the interface for object storage operations
type ObjectStorage interface {
	// UploadObject uploads an object
	UploadObject(objectKey string, reader io.Reader, size int64) error

	// DownloadObject downloads an object
	DownloadObject(objectKey string) ([]byte, error)

	// Exists checks if an object exists
	Exists(objectKey string) (bool, error)

	// ListObjects lists objects with a prefix
	ListObjects(prefix string, recursive bool) ([]string, error)

	// UploadFile uploads a file
	UploadFile(objectKey string, filePath string) error

	// DownloadFile downloads an object to a file
	DownloadFile(objectKey string, filePath string) error
}

// RangeObjectStorage is an optional extension for consumers that can process
// an object incrementally.  Keeping it separate preserves compatibility with
// existing ObjectStorage implementations while allowing independently framed
// compressed payloads to overlap range GETs with decoding.
type RangeObjectStorage interface {
	OpenObjectRange(ctx context.Context, objectKey string, offset, length int64) (io.ReadCloser, error)
}

// ContextObjectStorage opens small metadata objects with the restore's deadline.
// Large compressed payloads must use RangeObjectStorage instead.
type ContextObjectStorage interface {
	OpenObject(ctx context.Context, objectKey string) (io.ReadCloser, error)
}
