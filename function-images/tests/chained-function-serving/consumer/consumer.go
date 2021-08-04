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
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strconv"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	pb "tests/chained-functions-serving/proto"
	pb_client "tests/chained-functions-serving/proto"

	tracing "github.com/ease-lab/vhive/utils/tracing/go"

	sdk "github.com/ease-lab/vhive-xdt/sdk/golang"
	"github.com/ease-lab/vhive-xdt/utils"
)

const (
	INLINE = "INLINE"
	XDT = "XDT"
	S3 = "S3"
	AWS_S3_BUCKET = "vhive-prodcon-bench"
	TOKEN         = ""
)

var (
	AKID          string
	SECRET_KEY    string
	AWS_S3_REGION string
	S3_SVC *s3.S3
)

type consumerServer struct {
	transferType string
	pb.UnimplementedProducerConsumerServer
}

type ubenchServer struct {
	transferType string
	XDTconfig utils.Config
	pb_client.UnimplementedProdConDriverServer
}

func setAWSCredentials() {
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
	fmt.Printf("USING AWS ID: %v", AKID)
	sessionInstance, err := session.NewSession(&aws.Config{
		Region:      aws.String(AWS_S3_REGION),
		Credentials: credentials.NewStaticCredentials(AKID, SECRET_KEY, TOKEN),
	})
	if err != nil {
		log.Fatalf("[consumer] Failed establish s3 session: %s", err)
	}
	S3_SVC = s3.New(sessionInstance)
}

func fetchFromS3(ctx context.Context, key string) (int, error) {
	span := tracing.Span{SpanName: "S3 get", TracerName: "S3 get - tracer"}
	ctx = span.StartSpan(ctx)
	defer span.EndSpan()
	log.Infof("[consumer] Fetching %s from S3", key)
	object, err := S3_SVC.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(AWS_S3_BUCKET),
		Key:    aws.String(key),
	})
	if err != nil {
		log.Errorf("Object %q not found in S3 bucket %q: %s", key, AWS_S3_BUCKET, err.Error())
		return 0, err
	}

	payload, err := ioutil.ReadAll(object.Body)
	if err != nil {
		log.Infof("Error reading object body: %v", err)
		return 0, err
	}

	return len(payload), nil
}

func (s *consumerServer) ConsumeByte(ctx context.Context, str *pb.ConsumeByteRequest) (*pb.ConsumeByteReply, error) {
	if tracing.IsTracingEnabled() {
		span1 := tracing.Span{SpanName: "custom-span-1", TracerName: "tracer"}
		span2 := tracing.Span{SpanName: "custom-span-2", TracerName: "tracer"}
		ctx = span1.StartSpan(ctx)
		ctx = span2.StartSpan(ctx)
		defer span1.EndSpan()
		defer span2.EndSpan()
	}
	if s.transferType == S3 {
		payloadSize, err := fetchFromS3(ctx, string(str.Value))
		if err !=nil {
			return &pb.ConsumeByteReply{Value: false}, err
		}else {
			log.Printf("[consumer] Consumed %d bytes\n", payloadSize)
			return &pb.ConsumeByteReply{Value: true}, err
		}
	}else if s.transferType == INLINE{
		log.Printf("[consumer] Consumed %d bytes\n", len(str.Value))
	}
	return &pb.ConsumeByteReply{Value: true}, nil
}

func (s *consumerServer) ConsumeStream(stream pb.ProducerConsumer_ConsumeStreamServer) error {
	for {
		str, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.ConsumeByteReply{Value: true})
		}
		if err != nil {
			return err
		}
		log.Printf("[consumer] Consumed string of length %d\n", len(str.Value))
	}
}

func main() {
	port := flag.Int("ps", 80, "Port")
	url := flag.String("zipkin", "http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans", "zipkin url")
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	log.SetOutput(os.Stdout)

	if tracing.IsTracingEnabled() {
		log.Println("consumer has tracing enabled")
		shutdown, err := tracing.InitBasicTracer(*url, "consumer")
		if err != nil {
			log.Warn(err)
		}
		defer shutdown()
	} else {
		log.Println("consumer has tracing DISABLED")
	}

	transferType, ok := os.LookupEnv("TRANSFER_TYPE")
	if !ok {
		log.Infof("TRANSFER_TYPE not found, using INLINE transfer")
		transferType = "INLINE"
	}

	if transferType == S3 {
		setAWSCredentials()
	}

	var fanIn = false
	if value, ok := os.LookupEnv("FANIN"); ok{
		if intValue, err := strconv.Atoi(value); err==nil && intValue > 0{
			fanIn = true
		}
	}

	if transferType == XDT && !fanIn {
		var handler = func(data []byte) ([]byte, bool){
			log.Infof("gx: destination handler received data of size %d", len(data))
			return nil, true
		}
		config := utils.ReadConfig()
		sdk.StartDstServer(config, handler)
	} else {
		//set up server
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
		if err != nil {
			log.Fatalf("[consumer] failed to listen: %v", err)
		}

		var grpcServer *grpc.Server
		if tracing.IsTracingEnabled() {
			grpcServer = tracing.GetGRPCServerWithUnaryInterceptor()
		} else {
			grpcServer = grpc.NewServer()
		}
		cs := consumerServer{transferType: transferType}
		pb.RegisterProducerConsumerServer(grpcServer, &cs)
		us := ubenchServer{transferType: transferType,XDTconfig: utils.ReadConfig()}
		pb_client.RegisterProdConDriverServer(grpcServer, &us)

		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("[consumer] failed to serve: %s", err)
		}
	}
}
