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
	"bytes"
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"net"
	"os"

	sdk "github.com/ease-lab/vhive-xdt/sdk/golang"
	"github.com/ease-lab/vhive-xdt/utils"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/reflection"

	pb_client "tests/chained-functions-serving/proto"

	pb "github.com/ease-lab/vhive/examples/protobuf/helloworld"
	"google.golang.org/grpc"

	tracing "github.com/ease-lab/vhive/utils/tracing/go"
)

type producerServer struct {
	consumerAddr string
	consumerPort int
	payloadData  []byte
	XDTclient    sdk.XDTclient
	transferType string
	pb.UnimplementedGreeterServer
}

type ubenchServer struct {
	consumerAddr string
	consumerPort int
	transferType string
	payloadData  []byte
	XDTclient    sdk.XDTclient
	pb_client.UnimplementedProdConDriverServer
}

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
)

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
}

func uploadToS3(ctx context.Context, payloadData []byte) string {
	span := tracing.Span{SpanName: "S3 put", TracerName: "S3 put - tracer"}
	ctx = span.StartSpan(ctx)
	defer span.EndSpan()
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(AWS_S3_REGION),
		Credentials: credentials.NewStaticCredentials(AKID, SECRET_KEY, TOKEN),
	})
	if err != nil {
		log.Fatalf("[producer] Failed establish s3 session: %s", err)
	}
	log.Infof("[producer] uploading %d bytes to s3", len(payloadData))
	uploader := s3manager.NewUploader(sess)
	reader := bytes.NewReader(payloadData)
	key := "payload_bytes.txt"
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(AWS_S3_BUCKET),
		Key:    aws.String(key),
		Body:   reader,
	})
	log.Infof("[producer] S3 upload complete")
	if err != nil {
		log.Fatalf("[producer] Failed to upload bytes to s3: %s", err)
	}
	return key
}
func getGRPCclient(addr string) (pb_client.ProducerConsumerClient, *grpc.ClientConn) {
	// establish a connection
	var conn *grpc.ClientConn
	var err error
	if tracing.IsTracingEnabled() {
		conn, err = tracing.DialGRPCWithUnaryInterceptor(addr, grpc.WithBlock(), grpc.WithInsecure())
	} else {
		conn, err = grpc.Dial(addr, grpc.WithBlock(), grpc.WithInsecure())
	}
	if err != nil {
		log.Fatalf("[producer] fail to dial: %s", err)
	}
	return pb_client.NewProducerConsumerClient(conn), conn
}

func (ps *producerServer) SayHello(ctx context.Context, req *pb.HelloRequest) (_ *pb.HelloReply, err error) {
	addr := fmt.Sprintf("%v:%v", ps.consumerAddr, ps.consumerPort)
	if ps.transferType == INLINE || ps.transferType == S3 {
		client, conn := getGRPCclient(addr)
		defer conn.Close()
		payloadToSend := ps.payloadData
		if ps.transferType == S3 {
			payloadToSend = []byte(uploadToS3(ctx, ps.payloadData))
		}
		ack, err := client.ConsumeByte(ctx, &pb_client.ConsumeByteRequest{Value: payloadToSend})
		if err != nil {
			log.Fatalf("[producer] client error in string consumption: %s", err)
		}
		log.Printf("[producer] (single) Ack: %v\n", ack.Value)
	} else if ps.transferType == XDT {
		payloadToSend := utils.Payload{
			FunctionName: "HelloXDT",
			Data:         ps.payloadData,
		}
		if _, _, err := ps.XDTclient.Invoke(addr, payloadToSend); err != nil {
			log.Fatalf("SQP_to_dQP_data_transfer failed %v", err)
		}
	}
	return &pb.HelloReply{Message: "Success"}, err
}

func main() {
	flagAddress := flag.String("addr", "consumer.default.192.168.1.240.sslip.io", "Server IP address")
	flagClientPort := flag.Int("pc", 80, "Client Port")
	flagServerPort := flag.Int("ps", 80, "Server Port")
	url := flag.String("zipkin", "http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans", "zipkin url")
	transferSize := flag.Int("transferSize", 4095, "Number of KB's to transfer")
	dockerCompose := flag.Bool("dockerCompose", false, "Env docker Compose?")
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	log.SetOutput(os.Stdout)

	if tracing.IsTracingEnabled() {
		log.Println("producer has tracing enabled")
		shutdown, err := tracing.InitBasicTracer(*url, "producer")
		if err != nil {
			log.Warn(err)
		}
		defer shutdown()
	} else {
		log.Println("producer has tracing DISABLED")
	}

	var grpcServer *grpc.Server
	if tracing.IsTracingEnabled() {
		grpcServer = tracing.GetGRPCServerWithUnaryInterceptor()
	} else {
		grpcServer = grpc.NewServer()
	}

	reflection.Register(grpcServer)

	//client setup
	log.Printf("[producer] Client using address: %v:%d\n", *flagAddress, *flagClientPort)

	ps := producerServer{consumerAddr: *flagAddress, consumerPort: *flagClientPort}
    us := ubenchServer{consumerAddr: *flagAddress, consumerPort: *flagClientPort}

	transferType, ok := os.LookupEnv("TRANSFER_TYPE")
	if !ok {
		log.Infof("TRANSFER_TYPE not found, using INLINE transfer")
		transferType = INLINE
	}
	log.Infof("[producer] transfering via %s",transferType)
	ps.transferType = transferType
	us.transferType = transferType
	// 4194304 bytes is the limit by gRPC
	payloadData := make([]byte, *transferSize*1024) // 10MiB
	if _, err := rand.Read(payloadData); err != nil {
		log.Fatal(err)
	}
	log.Infof("sending %d bytes to consumer", len(payloadData))
	ps.payloadData = payloadData
	us.payloadData = payloadData
	if transferType == XDT {
		config := utils.ReadConfig()
		if !*dockerCompose {
			config.SQPServerHostname = utils.FetchSelfIP()
		}
		xdtClient, err := sdk.NewXDTclient(config)
		if err != nil {
			log.Fatalf("InitXDT failed %v", err)
		}

		ps.XDTclient = xdtClient
		us.XDTclient = xdtClient
	}else if transferType == S3 {
		setAWSCredentials()
	}
	pb.RegisterGreeterServer(grpcServer, &ps)
	pb_client.RegisterProdConDriverServer(grpcServer, &us)

	//server setup
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *flagServerPort))
	if err != nil {
		log.Fatalf("[producer] failed to listen: %v", err)
	}

	log.Println("[producer] Server Started")

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("[producer] failed to serve: %s", err)
	}

}
