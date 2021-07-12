package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb_video "tests/video_analytics/proto"

	pb_helloworld "github.com/ease-lab/vhive/examples/protobuf/helloworld"

	tracing "github.com/ease-lab/vhive/utils/tracing/go"
)

var videoFragment []byte

type server struct {
	decoderAddr string
	decoderPort int
	pb_helloworld.UnimplementedGreeterServer
}

// SayHello implements the helloworld interface. Used to trigger the video generator to start the benchmark.
func (s *server) SayHello(ctx context.Context, req *pb_helloworld.HelloRequest) (*pb_helloworld.HelloReply, error) {
	// Become a client of the decoder function and send the video:
	// establish a connection
	addr := fmt.Sprintf("%v:%v", s.decoderAddr, s.decoderPort)
	log.Infof("Using addr: %v", addr)
	conn, err := tracing.DialGRPCWithUnaryInterceptor(addr, grpc.WithBlock(), grpc.WithInsecure())
	if err != nil {
		log.Fatalf("[Video Generator] Failed to dial decoder: %s", err)
	}
	defer conn.Close()

	client := pb_video.NewDecodeVideoClient(conn)

	// send message
	log.Infof("Vide Fragment length: %v", len(videoFragment))
	reply, err := client.SendVideo(ctx, &pb_video.SendVideoRequest{Value: videoFragment})
	if err != nil {
		log.Fatalf("[Video Generator] Failed to send video to decoder: %s", err)
	}
	log.Infof("[Video Generator] Decoder replied: %v\n", reply.Value)
	return &pb_helloworld.HelloReply{Message: reply.Value}, err
}

func main() {
	debug := flag.Bool("d", false, "Debug level in logs")
	decoderAddr := flag.String("addr", "decoder.default.192.168.1.240.sslip.io", "Decoder address")
	decoderPort := flag.Int("p", 80, "Decoder port")
	servePort := flag.Int("sp", 80, "Port listened to by this generator")
	videoFile := flag.String("video", "reference/video.mp4", "The file location of the video")
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

	shutdown, err := tracing.InitBasicTracer(*zipkin, "Video Generator")
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
	grpcServer := tracing.GetGRPCServerWithUnaryInterceptor()
	reflection.Register(grpcServer)
	server := server{}
	server.decoderAddr = *decoderAddr
	server.decoderPort = *decoderPort
	pb_helloworld.RegisterGreeterServer(grpcServer, &server)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
