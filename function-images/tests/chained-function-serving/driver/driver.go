package main

import (
	"context"
	"flag"
	"fmt"
	"google.golang.org/grpc/reflection"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	pb_client "tests/chained-functions-serving/proto"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	pb "github.com/ease-lab/vhive/examples/protobuf/helloworld"
	tracing "github.com/ease-lab/vhive/utils/tracing/go"
)

const(
	FANOUT = "FANOUT"
	FANIN = "FANIN"
	BROADCAST = "BROADCAST"
)

var (
	consPort    *int
	prodPort    *int
	grpcTimeout time.Duration
	withTracing *bool
)

type driverServer struct {
	prodEndpoint string
	consEndpoint string
	consPort int
	prodPort int
	fanIn int
	fanOut int
	pb.UnimplementedGreeterServer
}

func (s *driverServer) SayHello(ctx context.Context, req *pb.HelloRequest) (_ *pb.HelloReply, err error) {
	prodAddr := fmt.Sprintf("%s:%d",s.prodEndpoint,s.prodPort)
	consAddr := fmt.Sprintf("%s:%d",s.consEndpoint,s.consPort)
	span := tracing.Span{SpanName: "Driver", TracerName: "Driver - tracer"}
	ctx = span.StartSpan(ctx)
	defer span.EndSpan()
	invokeServingFunction(ctx, prodAddr, consAddr, s.fanIn, s.fanOut)
	return &pb.HelloReply{Message: "Success"}, err
}

func setEnv(server *driverServer, fanIn, fanOut int, prodEndpoint, consEndpoint string, prodPort, consPort int){
	server.fanIn = fanIn
	if value, ok := os.LookupEnv("FANIN"); ok {
		server.fanIn, _ = strconv.Atoi(value)
	}
	server.fanOut = fanOut
	if value, ok := os.LookupEnv("FANOUT"); ok {
		server.fanOut, _ = strconv.Atoi(value)
	}
	server.prodEndpoint = prodEndpoint
	if value, ok := os.LookupEnv("PROD_ENDPOINT"); ok {
		server.prodEndpoint = value
	}
	server.consEndpoint = consEndpoint
	if value, ok := os.LookupEnv("CONS_ENDPOINT"); ok {
		server.consEndpoint = value
	}
	server.prodPort = prodPort
	if value, ok := os.LookupEnv("PROD_PORT"); ok {
		if intValue, err := strconv.Atoi(value); err!=nil {
			server.prodPort = intValue
		}
	}
	server.consPort = consPort
	if value, ok := os.LookupEnv("CONS_PORT"); ok {
		if intValue, err := strconv.Atoi(value); err!=nil {
			server.consPort = intValue
		}
	}
}

func main() {
	prodEndpoint := flag.String("prodEndpoint", "producer.default.192.168.1.240.sslip.io", "Endpoint to ping")
	prodPort = flag.Int("prodPort", 80, "The port that the producer is listening to")
	consEndpoint := flag.String("consEndpoint", "consumer.default.192.168.1.240.sslip.io", "Endpoint to ping")
	consPort = flag.Int("consPort", 80, "The port that the consumer is listening to")
	servePort := flag.Int("sp", 80, "Port listened to by this streamer")
	withTracing = flag.Bool("trace", false, "Enable tracing in the client")
	zipkin := flag.String("zipkin", "http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans", "zipkin url")
	debug := flag.Bool("dbg", false, "Enable debug logging")
	grpcTimeout = time.Duration(*flag.Int("grpcTimeout", 60, "Timeout in seconds for gRPC requests")) * time.Second
	fanIn := flag.Int("fanIn",0,"Fan in amount")
	fanOut := flag.Int("fanOut",0,"Fan out amount")

	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	log.SetOutput(os.Stdout)
	if strings.EqualFold(os.Getenv("DEBUG"), "true") || *debug {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging is enabled")
	} else {
		log.SetLevel(log.InfoLevel)
	}

	var grpcServer *grpc.Server
	if strings.EqualFold(os.Getenv("ENABLE_TRACING"), "true") || *withTracing {
		*withTracing = true
		shutdown, err := tracing.InitBasicTracer(*zipkin, "driver")
		if err != nil {
			log.Print(err)
		}
		defer shutdown()
		grpcServer = tracing.GetGRPCServerWithUnaryInterceptor()
	}else {
		grpcServer = grpc.NewServer()
	}
	server := driverServer{}
	setEnv(&server, *fanIn, *fanOut, *prodEndpoint, *consEndpoint, *prodPort, *consPort)
	pb.RegisterGreeterServer(grpcServer, &server)
	reflection.Register(grpcServer)

	// server setup: listen on port.
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *servePort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func SayHello(ctx context.Context, address string) {
	dialOptions := []grpc.DialOption{grpc.WithBlock(), grpc.WithInsecure()}
	if *withTracing {
		dialOptions = append(dialOptions, grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	}
	conn, err := grpc.Dial(address, dialOptions...)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := pb.NewGreeterClient(conn)

	ctx, cancel := context.WithTimeout(ctx, grpcTimeout)
	defer cancel()

	_, err = c.SayHello(ctx, &pb.HelloRequest{Name: "faas"})
	if err != nil {
		log.Warnf("Failed to invoke %v, err=%v", address, err)
	}
}

func benchFanIn(ctx context.Context, prodAddr, consAddr string, fanInAmount int) {
	dialOptions := []grpc.DialOption{grpc.WithBlock(), grpc.WithInsecure()}
	if *withTracing {
		dialOptions = append(dialOptions, grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	}
	conn, err := grpc.Dial(prodAddr, dialOptions...)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := pb_client.NewProdConDriverClient(conn)

	ctx, cancel := context.WithTimeout(ctx, grpcTimeout)
	defer cancel()
	var benchResponse *pb_client.BenchResponse
	var capabilities []string
	errorChannel := make(chan error, fanInAmount)
	for i :=0; i<fanInAmount; i++ {
		go func() {
			benchResponse, err = c.Benchmark(ctx, &pb_client.BenchType{Name: FANIN, FanAmount: int64(fanInAmount)})
			if err != nil {
				log.Warnf("Failed to invoke %s, err=%v", prodAddr, err)
				errorChannel <- err
			}else{
				log.Infof("[driver] Push successful")
				errorChannel <- nil
			}
			capabilities = append(capabilities, benchResponse.Capability)
		}()
	}
	for i :=0; i<fanInAmount; i++ {
		select {
		case err := <-errorChannel:
			if err != nil {
				log.Errorf("[driver] FanIn push failed: %v",err)
				return
			}
		}
	}
	log.Infof("received capabilites %v",capabilities)
	reduce(ctx, consAddr, capabilities)
}

func reduce(ctx context.Context, consEndpoint string, capabilities []string) {
	log.Infof("Attempting reduction using addr:%s",consEndpoint)
	dialOptions := []grpc.DialOption{grpc.WithBlock(), grpc.WithInsecure()}
	if *withTracing {
		dialOptions = append(dialOptions, grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	}
	conn, err := grpc.Dial(consEndpoint, dialOptions...)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := pb_client.NewProdConDriverClient(conn)

	ctx, cancel := context.WithTimeout(ctx, grpcTimeout)
	defer cancel()
	_, err = c.FetchByte(ctx, &pb_client.ReductionRequest{Capability: capabilities})
	if err != nil {
		log.Errorf("Fetch@consumer failed %v",err)
	}
}

func benchFanOut(ctx context.Context, prodAddr string, fanOutAmount int) {
	dialOptions := []grpc.DialOption{grpc.WithBlock(), grpc.WithInsecure()}
	if *withTracing {
		dialOptions = append(dialOptions, grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	}
	conn, err := grpc.Dial(prodAddr, dialOptions...)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := pb_client.NewProdConDriverClient(conn)

	ctx, cancel := context.WithTimeout(ctx, grpcTimeout)
	defer cancel()

	_, err = c.Benchmark (ctx, &pb_client.BenchType{Name: FANOUT, FanAmount: int64(fanOutAmount)})
	if err != nil {
		log.Warnf("Failed to invoke %v, err=%v", prodAddr, err)
	}
}

func invokeServingFunction(ctx context.Context, prodAddr, consAddr string, fanInAmount, fanOutAmount int) {

	log.Debug("Invoking by the address: %v", prodAddr)

	if fanInAmount > 0 {
		benchFanIn(ctx, prodAddr, consAddr, fanInAmount)
	}else if fanOutAmount > 0 {
		benchFanOut(ctx, prodAddr, fanOutAmount)
	}else {
		SayHello(ctx, prodAddr)
	}

	return
}
