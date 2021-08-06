// MIT License
//
// Copyright (c) 2021 Michal Baczun and EASE lab
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

// Package tracing provides a simple utility for including opentelemetry and zipkin tracing
// instrumentation in vHive and Knative workloads

package storage

import (
	"bytes"
	"context"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/go-redis/redis/v8"
	log "github.com/sirupsen/logrus"
)

const (
	TOKEN       = ""
	XDT         = "XDT"
	S3          = "S3"
	ELASTICACHE = "ELASTICACHE"
)

var (
	transferType  string
	benchName     string
	AKID          string
	SECRET_KEY    string
	AWS_S3_REGION string
	redisClient   *redis.Client
	s3session     *session.Session
)
var ctx = context.Background()

// InitStorage initialises the storage modue. This function is used to provide information about
// which service to use. If s3 is used, "bucket" is the bucket used for storage, and in the case
// when elasticache is used "bucket" should be the redis URL and port.
// Note that one must be on an AWS VPC (e.g. using EC2) to access elasticache.
func InitStorage(service string, bucket string) {
	transferType = service
	benchName = bucket
	awsAccessKey, ok := os.LookupEnv("AWS_ACCESS_KEY")
	if ok {
		AKID = awsAccessKey
	}
	awsSecretKey, ok := os.LookupEnv("AWS_SECRET_KEY")
	if ok {
		SECRET_KEY = awsSecretKey
	}
	AWS_S3_REGION = "us-west-1"
	awsRegion, ok := os.LookupEnv("AWS_REGION")
	if ok {
		AWS_S3_REGION = awsRegion
	}
	if transferType == S3 {
		var err error
		s3session, err = session.NewSession(&aws.Config{
			Region:      aws.String(AWS_S3_REGION),
			Credentials: credentials.NewStaticCredentials(AKID, SECRET_KEY, TOKEN),
		})
		if err != nil {
			log.Fatalf("Failed establish s3 session: %s", err)
		}
	} else if transferType == ELASTICACHE {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     benchName,
			Password: "", // no password set
			DB:       0,  // use default DB
		})
	}
}

// Put uploads the payload to the storage service using the provided key
func Put(key string, payloadData []byte) string {
	if transferType == S3 {
		log.Infof("Uploading %d bytes to s3", len(payloadData))
		uploader := s3manager.NewUploader(s3session)
		reader := bytes.NewReader(payloadData)
		_, err := uploader.Upload(&s3manager.UploadInput{
			Bucket: aws.String(benchName),
			Key:    aws.String(key),
			Body:   reader,
		})
		log.Infof("S3 upload complete")
		if err != nil {
			log.Fatalf("Failed to upload bytes to s3: %s", err)
		}
		return key

	} else if transferType == ELASTICACHE {
		log.Infof("Uploading %d bytes to ElastiCache", len(payloadData))
		err := redisClient.Set(ctx, key, payloadData, 0).Err()
		if err != nil {
			panic(err)
		}
		return key
	} else {
		log.Fatalf("Unsupported transfer type: %s", transferType)
	}
	return ""
}

// Put uploads the payload to the storage service using the provided key
func PutFile(key string, file *os.File) string {
	if transferType == S3 {
		log.Infof("Uploading file to s3")
		uploader := s3manager.NewUploader(s3session)
		_, err := uploader.Upload(&s3manager.UploadInput{
			Bucket: aws.String(benchName),
			Key:    aws.String(key),
			Body:   file,
		})
		log.Infof("S3 upload complete")
		if err != nil {
			log.Fatalf("Failed to upload bytes to s3: %s", err)
		}
		return key

	} else if transferType == ELASTICACHE {
		log.Fatalf("File transfer via ElastiCahce currently unsupported, please use []bytes: `Put(key string, payloadData []byte) string`")
	} else {
		log.Fatalf("Unsupported transfer type: %s", transferType)
	}
	return ""
}

// Get retrieves a payload corresponding to the provided key from the storage service.
// An error will occur if the key is not prescent on the service.
func Get(key string) []byte {
	if transferType == S3 {
		log.Infof("Fetching %s from S3", key)
		downloader := s3manager.NewDownloader(s3session)
		buf := aws.NewWriteAtBuffer([]byte{})
		_, err := downloader.Download(buf, &s3.GetObjectInput{
			Bucket: aws.String(benchName),
			Key:    aws.String(key)})
		if err != nil {
			log.Fatalf("[Failed to fetch bytes from s3: %s", err)
		}
		return buf.Bytes()

	} else if transferType == ELASTICACHE {
		log.Infof("Fetching %s from ElastiCache", key)
		val, err := redisClient.Get(ctx, key).Result()
		if err != nil {
			panic(err)
		}
		return []byte(val)
	} else {
		log.Fatalf("Unsupported transfer type: %s", transferType)
	}
	return nil
}
