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
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/minio/minio-go/v7"
	"github.com/pkg/errors"
)

type MinioStorage struct {
	client     *minio.Client
	bucketName string
	fetchStats [remoteFetchClassCount]remoteFetchCounter
}

const (
	remoteFetchChunks = iota
	remoteFetchWorkingSetPayload
	remoteFetchRecipe
	remoteFetchWorkingSetMetadata
	remoteFetchSnapshotMetadata
	remoteFetchSharedWorkingSet
	remoteFetchOther
	remoteFetchClassCount
)

var remoteFetchClassNames = [remoteFetchClassCount]string{
	"chunks",
	"working_set_payload",
	"recipe",
	"working_set_metadata",
	"snapshot_metadata",
	"shared_working_set",
	"other",
}

type remoteFetchCounter struct {
	requests atomic.Uint64
	bytes    atomic.Uint64
}

type countingReadCloser struct {
	inner   io.ReadCloser
	counter *remoteFetchCounter
	once    sync.Once
}

func (reader *countingReadCloser) Read(buffer []byte) (int, error) {
	n, err := reader.inner.Read(buffer)
	if n > 0 {
		reader.once.Do(func() { reader.counter.requests.Add(1) })
		reader.counter.bytes.Add(uint64(n))
	}
	return n, err
}

func (reader *countingReadCloser) Close() error {
	return reader.inner.Close()
}

func remoteFetchClass(objectKey string) int {
	base := objectKey
	if slash := strings.LastIndexByte(objectKey, '/'); slash >= 0 {
		base = objectKey[slash+1:]
	}
	switch {
	case strings.HasPrefix(objectKey, "_chunks_zstd_v1/") ||
		strings.HasPrefix(objectKey, "_chunks_zstd_v1_l") ||
		strings.Contains(objectKey, "/_chunks_zstd_v1/") ||
		strings.Contains(objectKey, "/_chunks_zstd_v1_l"):
		return remoteFetchChunks
	case strings.Contains(base, "working_set_pages_content") &&
		(strings.HasSuffix(base, ".zstd.frames") || strings.HasSuffix(base, ".zstd.streams")):
		return remoteFetchWorkingSetPayload
	case base == "recipe_file":
		return remoteFetchRecipe
	case base == "working_set_pages" ||
		strings.HasPrefix(base, "working_set_pages_index") ||
		(strings.Contains(base, "working_set_pages_content") &&
			(strings.HasSuffix(base, ".zstd.json") || strings.HasSuffix(base, ".zstd.streams.json"))):
		return remoteFetchWorkingSetMetadata
	case base == "snap_file" || base == "info_file":
		return remoteFetchSnapshotMetadata
	case strings.HasPrefix(objectKey, "ws_shared/") || strings.Contains(objectKey, "/ws_shared/"):
		return remoteFetchSharedWorkingSet
	default:
		return remoteFetchOther
	}
}

func (m *MinioStorage) countCompletedObjectRead(objectKey string, bytes uint64) {
	counter := &m.fetchStats[remoteFetchClass(objectKey)]
	counter.requests.Add(1)
	counter.bytes.Add(bytes)
}

func (m *MinioStorage) SnapshotRemoteFetchStats() RemoteFetchStats {
	stats := RemoteFetchStats{Classes: make(map[string]RemoteFetchClassStats, remoteFetchClassCount)}
	for index, name := range remoteFetchClassNames {
		entry := RemoteFetchClassStats{
			Requests: m.fetchStats[index].requests.Load(),
			Bytes:    m.fetchStats[index].bytes.Load(),
		}
		stats.Classes[name] = entry
		stats.Total.Requests += entry.Requests
		stats.Total.Bytes += entry.Bytes
	}
	return stats
}

func (m *MinioStorage) ResetRemoteFetchStats() {
	for index := range m.fetchStats {
		m.fetchStats[index].requests.Store(0)
		m.fetchStats[index].bytes.Store(0)
	}
}

func NewMinioStorage(client *minio.Client, bucketName string) (*MinioStorage, error) {
	// Ensure bucket exists
	exists, err := client.BucketExists(context.Background(), bucketName)
	if err != nil {
		return nil, errors.Wrap(err, "checking bucket existence")
	}
	if !exists {
		err = client.MakeBucket(context.Background(), bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "creating bucket")
		}
	}
	return &MinioStorage{client: client, bucketName: bucketName}, nil
}

func (m *MinioStorage) UploadObject(objectKey string, reader io.Reader, size int64) error {
	_, err := m.client.PutObject(
		context.Background(),
		m.bucketName,
		objectKey,
		reader,
		size,
		minio.PutObjectOptions{},
	)
	return errors.Wrapf(err, "uploading object %s", objectKey)
}

func (m *MinioStorage) DownloadObject(objectKey string) ([]byte, error) {
	stat, err := m.client.StatObject(
		context.Background(),
		m.bucketName,
		objectKey,
		minio.StatObjectOptions{},
	)
	if err != nil {
		return nil, errors.Wrapf(err, "stat object %s", objectKey)
	}
	if stat.Size < 0 {
		return nil, fmt.Errorf("object %s has negative size %d", objectKey, stat.Size)
	}
	if stat.Size == 0 {
		return []byte{}, nil
	}

	data := make([]byte, stat.Size)

	concurrency := 10
	if stat.Size < 1024*1024 { // For small objects, use single connection to avoid overhead
		concurrency = 1
	}

	errCh := make(chan error, concurrency)
	var wg sync.WaitGroup
	for i := range concurrency {
		wg.Add(1)
		go func(rangeIndex int) {
			defer wg.Done()

			start := int64(rangeIndex) * (stat.Size / int64(concurrency))
			end := start + (stat.Size / int64(concurrency))
			if rangeIndex == concurrency-1 {
				end = stat.Size
			}
			obj, err := m.OpenObjectRange(context.Background(), objectKey, start, end-start)
			if err != nil {
				errCh <- err
				return
			}
			defer obj.Close()

			if _, err := io.ReadFull(obj, data[start:end]); err != nil {
				errCh <- errors.Wrapf(err, "read object %s range [%d,%d)", objectKey, start, end)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errCh)
	for rangeErr := range errCh {
		if rangeErr != nil {
			return nil, rangeErr
		}
	}
	return data, nil
}

// OpenObjectRange opens one byte range without buffering it.  MinIO performs
// the request as the returned reader is consumed, so a Zstandard decoder can
// start producing output before the entire range has arrived.
func (m *MinioStorage) OpenObjectRange(ctx context.Context, objectKey string, offset, length int64) (io.ReadCloser, error) {
	if offset < 0 {
		return nil, fmt.Errorf("object %s range has negative offset %d", objectKey, offset)
	}
	if length <= 0 {
		return nil, fmt.Errorf("object %s range has non-positive length %d", objectKey, length)
	}
	end := offset + length - 1
	if end < offset {
		return nil, fmt.Errorf("object %s range overflows: offset=%d length=%d", objectKey, offset, length)
	}

	opts := minio.GetObjectOptions{}
	if err := opts.SetRange(offset, end); err != nil {
		return nil, errors.Wrapf(err, "set object %s range [%d,%d]", objectKey, offset, end)
	}
	obj, err := m.client.GetObject(ctx, m.bucketName, objectKey, opts)
	if err != nil {
		return nil, errors.Wrapf(err, "open object %s range [%d,%d]", objectKey, offset, end)
	}
	return &countingReadCloser{
		inner:   obj,
		counter: &m.fetchStats[remoteFetchClass(objectKey)],
	}, nil
}

func (m *MinioStorage) OpenObject(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	obj, err := m.client.GetObject(ctx, m.bucketName, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return &countingReadCloser{inner: obj, counter: &m.fetchStats[remoteFetchClass(objectKey)]}, nil
}

func (m *MinioStorage) Exists(objectKey string) (bool, error) {
	_, err := m.client.StatObject(
		context.Background(),
		m.bucketName,
		objectKey,
		minio.StatObjectOptions{},
	)
	if err != nil {
		// Check if the error is because the object doesn't exist
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, errors.Wrapf(err, "checking if object %s exists", objectKey)
	}
	return true, nil
}

func (m *MinioStorage) ListObjects(prefix string, recursive bool) ([]string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	objectCh := m.client.ListObjects(ctx, m.bucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: recursive,
	})

	var objects []string
	for object := range objectCh {
		if object.Err != nil {
			return nil, object.Err
		}
		objects = append(objects, object.Key)
	}
	return objects, nil
}

func (m *MinioStorage) UploadFile(objectKey string, filePath string) error {
	_, err := m.client.FPutObject(
		context.Background(),
		m.bucketName,
		objectKey,
		filePath,
		minio.PutObjectOptions{},
	)
	return errors.Wrapf(err, "uploading file %s", filePath)
}

func (m *MinioStorage) DownloadFile(objectKey string, filePath string) error {
	err := m.client.FGetObject(
		context.Background(),
		m.bucketName,
		objectKey,
		filePath,
		minio.GetObjectOptions{},
	)
	if err != nil {
		return errors.Wrapf(err, "downloading object %s to %s", objectKey, filePath)
	}
	stat, err := os.Stat(filePath)
	if err != nil {
		return errors.Wrapf(err, "stat downloaded object %s at %s", objectKey, filePath)
	}
	m.countCompletedObjectRead(objectKey, uint64(stat.Size()))
	return nil
}
