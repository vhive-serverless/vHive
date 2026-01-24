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

	"github.com/minio/minio-go/v7"
	"github.com/pkg/errors"
)

type MinioStorage struct {
	client     *minio.Client
	bucketName string
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

func (m *MinioStorage) DownloadObject(objectKey string) (io.ReadCloser, error) {
	obj, err := m.client.GetObject(
		context.Background(),
		m.bucketName,
		objectKey,
		minio.GetObjectOptions{},
	)
	if err != nil {
		return nil, errors.Wrapf(err, "getting object %s", objectKey)
	}
	return obj, nil
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
