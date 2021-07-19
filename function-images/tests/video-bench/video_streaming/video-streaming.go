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

package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	pb_video "tests/video_analytics/proto"

	pb_helloworld "github.com/ease-lab/vhive/examples/protobuf/helloworld"

	tracing "github.com/ease-lab/vhive/utils/tracing/go"
)

var (
	videoFragment []byte
	videoFile     *string
	AKID          string
	SECRET_KEY    string
)

const (
	AWS_S3_REGION = "us-west-1"
	AWS_S3_BUCKET = "vhive-video-bench"

	TOKEN = ""
)

type server struct {
	decoderAddr string
	decoderPort int
	pb_helloworld.UnimplementedGreeterServer
}

func setAWSCredentials() {
	aws_access_key, ok1 := os.LookupEnv("AWS_ACCESS_KEY")
	if ok1 {
		AKID = aws_access_key
	}
	aws_secret_key, ok2 := os.LookupEnv("AWS_SECRET_KEY")
	if ok2 {
		SECRET_KEY = aws_secret_key
	}
	fmt.Printf("USING AWS ID: %v", AKID)
}

// SayHello implements the helloworld interface. Used to trigger the video streamer to start the benchmark.
func (s *server) SayHello(ctx context.Context, req *pb_helloworld.HelloRequest) (_ *pb_helloworld.HelloReply, err error) {
	// Become a client of the decoder function and send the video:
	// establish a connection
	addr := fmt.Sprintf("%v:%v", s.decoderAddr, s.decoderPort)
	log.Infof("[Video Streaming] Using addr: %v", addr)

	var conn *grpc.ClientConn
	if tracing.IsTracingEnabled() {
		conn, err = tracing.DialGRPCWithUnaryInterceptor(addr, grpc.WithBlock(), grpc.WithInsecure())
	} else {
		conn, err = grpc.Dial(addr, grpc.WithBlock(), grpc.WithInsecure())
	}

	if err != nil {
		log.Fatalf("[Video Streaming] Failed to dial decoder: %s", err)
	}
	defer conn.Close()

	client := pb_video.NewVideoDecoderClient(conn)

	// send message
	log.Infof("[Video Streaming] Video Fragment length: %v", len(videoFragment))

	var uses3 bool
	if val, ok := os.LookupEnv("USES3"); !ok || val == "false" {
		uses3 = false
	} else if val == "true" {
		uses3 = true
	} else {
		log.Fatalf("Invalid USES3 value")
	}

	var reply *pb_video.DecodeReply
	if uses3 {
		// upload video to s3
		sess, err := session.NewSession(&aws.Config{
			Region:      aws.String(AWS_S3_REGION),
			Credentials: credentials.NewStaticCredentials(AKID, SECRET_KEY, TOKEN),
		})
		if err != nil {
			log.Fatalf("[Video Streaming] Failed establish s3 session: %s", err)
		}
		file, err := os.Open(*videoFile)
		if err != nil {
			log.Fatalf("[Video Streaming] Failed to open file: %s", err)
		}
		log.Infof("[Video Streaming] uploading video to s3")
		uploader := s3manager.NewUploader(sess)
		_, err = uploader.Upload(&s3manager.UploadInput{
			Bucket: aws.String(AWS_S3_BUCKET),
			Key:    aws.String("streaming-video.mp4"),
			Body:   file,
		})
		if err != nil {
			log.Fatalf("[Video Streaming] Failed to upload file to s3: %s", err)
		}
		log.Infof("[Video Streaming] Uploaded video to s3")
		// issue request
		reply, err = client.Decode(ctx, &pb_video.DecodeRequest{S3Key: "streaming-video.mp4"})

	} else {
		reply, err = client.Decode(ctx, &pb_video.DecodeRequest{Video: videoFragment})
	}
	if err != nil {
		log.Fatalf("[Video Streaming] Failed to send video to decoder: %s", err)
	}
	log.Infof("[Video Streaming] Received Decoder reply")
	return &pb_helloworld.HelloReply{Message: reply.Classification}, err
}

func main() {
	debug := flag.Bool("d", false, "Debug level in logs")
	decoderAddr := flag.String("addr", "decoder.default.192.168.1.240.sslip.io", "Decoder address")
	decoderPort := flag.Int("p", 80, "Decoder port")
	servePort := flag.Int("sp", 80, "Port listened to by this streamer")
	videoFile = flag.String("video", "reference/video.mp4", "The file location of the video")
	zipkin := flag.String("zipkin", "http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans", "zipkin url")
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})

	if *debug {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging is enabled")
	} else {
		log.SetLevel(log.InfoLevel)
	}

	setAWSCredentials()

	shutdown, err := tracing.InitBasicTracer(*zipkin, "Video Streaming")
	if err != nil {
		log.Warn(err)
	}
	defer shutdown()

	videoFragment, err = ioutil.ReadFile(*videoFile)
	log.Infof("read video fragment, size: %v\n", len(videoFragment))

	// server setup: listen on port.
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *servePort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	var grpcServer *grpc.Server
	if tracing.IsTracingEnabled() {
		grpcServer = tracing.GetGRPCServerWithUnaryInterceptor()
	} else {
		grpcServer = grpc.NewServer()
	}

	reflection.Register(grpcServer)
	server := server{}
	server.decoderAddr = *decoderAddr
	server.decoderPort = *decoderPort
	pb_helloworld.RegisterGreeterServer(grpcServer, &server)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
