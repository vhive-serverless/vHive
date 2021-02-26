// MIT License
//
// Copyright (c) 2021 Dmitrii Ustiugov and EASE lab
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
	"bytes"
	"context"
	"flag"
	"io"
	"net"
	"os"
	"strings"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"

	pb "github.com/ease-lab/vhive/function-images/tests/save_load_minio/proto"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"google.golang.org/grpc"
)

const (
	serverAddress = ":50051"
	minioBucket   = "mybucket"
)

var minioClientSingleton *minio.Client

func authenticateStorageClient(serverAddress string) *minio.Client {

	const (
		// MinIO credentials and address
		//serverAddress = "10.96.0.46:9000"
		accessKey = "minio"
		secretKey = "minio123"
	)
	if minioClientSingleton != nil {
		return minioClientSingleton
	}

	minioClient, err := minio.New(serverAddress, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false,
	})
	if err != nil {
		log.Fatalf("Could not create minio client: %s", err.Error())
	}

	minioClientSingleton = minioClient
	return minioClientSingleton
}

func loadObject(serverAddress, objectBucket, objectKey string) string {
	storageClient := authenticateStorageClient(serverAddress)
	object, err := storageClient.GetObject(
		context.Background(),
		objectBucket,
		objectKey,
		minio.GetObjectOptions{},
	)
	if err != nil {
		log.Infof("Object %q not found in bucket %q: %s", objectKey, objectBucket, err.Error())
	}

	var buf bytes.Buffer
	if _, err = io.Copy(&buf, object); err != nil {
		log.Infof("Error reading object body: %v", err)
		return ""
	}

	return buf.String()
}

func saveObject(serverAddress, payload, bucket string) string {
	//key := fmt.Sprintf("transfer-payload-%s", generateStringPayload(20))
	key := "transfer-payload"
	log.Infof(`Using storage, saving transfer payload (~%d bytes) as %q to %q.`, len(payload), key, bucket)

	storageClient := authenticateStorageClient(serverAddress)

	uploadOutput, err := storageClient.PutObject(
		context.Background(),
		bucket,
		key,
		strings.NewReader(payload),
		int64(len(payload)),
		minio.PutObjectOptions{},
	)
	if err != nil {
		log.Fatalf("Unable to upload %q to %q, %v", key, bucket, err.Error())
	}

	log.Infof("Successfully uploaded %q to bucket %q (%s)", key, bucket, uploadOutput.Location)
	return key
}

type server struct {
	pb.UnimplementedSaveAndLoadServer
}

func generateStringPayload(payloadLengthBytes int) string {
	const allowedChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

	generatedTransferPayload := make([]byte, payloadLengthBytes)
	//for i := range generatedTransferPayload {
	//	generatedTransferPayload[i] = allowedChars[rand.Intn(len(allowedChars))]
	//}

	return string(generatedTransferPayload)
}

func (s *server) Invoke(ctx context.Context, in *pb.InvokeRequest) (*pb.InvokeReply, error) {
	log.Infof("Received a request for the payload size of %v bytes", in.GetPayloadSize())

	if in.GetUseS3() {
		log.Info("Generating the payload")
		payload := generateStringPayload(int(in.GetPayloadSize()))

		log.Infof("Saving the payload")
		key := saveObject(in.GetMinioAddress(), payload, minioBucket)
		log.Infof("Saved the payload (key=%v)", key)

		log.Infof("Loading the payload (key=%v)", key)
		_ = loadObject(in.GetMinioAddress(), minioBucket, key)
		log.Infof("Loaded the payload (key=%v)", key)
	} else {
		log.Info("Using storage is disabled")
	}

	return &pb.InvokeReply{Ok: true}, nil
}

func main() {
	debug := flag.Bool("d", false, "Debug level in logs")
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})

	if file, err := os.OpenFile("/tmp/log.out", os.O_CREATE|os.O_WRONLY, 0666); err == nil {
		log.SetOutput(file)
	}

	if *debug {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging is enabled")
	} else {
		log.SetLevel(log.InfoLevel)
	}

	lis, err := net.Listen("tcp", serverAddress)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterSaveAndLoadServer(s, &server{})
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
