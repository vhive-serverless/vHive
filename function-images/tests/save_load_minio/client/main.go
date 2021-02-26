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
	"context"
	"flag"
	"os"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"

	pb "github.com/ease-lab/vhive/function-images/tests/save_load_minio/proto"
	"google.golang.org/grpc"
)

func main() {
	debug := flag.Bool("d", false, "Debug level in logs")
	useS3 := flag.Bool("s3", true, "Use S3 storage")
	payloadSize := flag.Int64("size", 8, "Payload size in bytes")
	address := flag.String("addr", "localhost:50051", "Function server address")
	minioAddress := flag.String("minioAddr", "localhost:50052", "MinIO server address (vHive: 10.96.0.46:9000)")
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})

	log.SetOutput(os.Stdout)

	if *debug {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging is enabled")
	} else {
		log.SetLevel(log.InfoLevel)
	}

	conn, err := grpc.Dial(*address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewSaveAndLoadClient(conn)

	log.Debug("Invoking the server")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := c.Invoke(ctx, &pb.InvokeRequest{
		UseS3:        *useS3,
		PayloadSize:  *payloadSize,
		MinioAddress: *minioAddress,
	})
	if err != nil {
		log.Fatalf("could not invoke: %v", err)
	}
	log.Debugf("Success: %v", r.GetOk())
}
