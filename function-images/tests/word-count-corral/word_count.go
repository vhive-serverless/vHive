package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/bcongdon/corral"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	tracing "github.com/ease-lab/vhive/utils/tracing/go"
)

type wordCount struct{}

func (w wordCount) Map(ctx context.Context, key, value string, emitter corral.Emitter) {
	re := regexp.MustCompile(`[^a-zA-Z0-9\s]+`)

	sanitized := strings.ToLower(re.ReplaceAllString(value, " "))
	for _, word := range strings.Fields(sanitized) {
		if len(word) == 0 {
			continue
		}
		err := emitter.Emit(ctx, word, strconv.Itoa(1))
		if err != nil {
			fmt.Println(err)
		}
	}
}

func (w wordCount) Reduce(ctx context.Context, key string, values corral.ValueIterator, emitter corral.Emitter) {
	count := 0
	for range values.Iter() {
		count++
	}
	emitter.Emit(ctx, key, strconv.Itoa(count))
}

func main() {
	if tracing.IsTracingEnabled() {
		shutdown, err := tracing.InitBasicTracer("http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans", "producer")
		if err != nil {
			logrus.Fatal("Failed to initialize tracing", err)
		}
		defer shutdown()
	}

	if os.Getenv("CORRAL_DRIVER") == "1" {
		driverMain()
	} else {
		workerMain()
	}
}

func workerMain() {
	job := corral.NewJob(wordCount{}, wordCount{})
	options := []corral.Option{
		corral.WithSplitSize(10 * 1024),
		corral.WithMapBinSize(10 * 1024),
	}
	driver := corral.NewDriver(job, options...)
	driver.Main(context.Background())
}

type server struct {
	UnimplementedGreeterServer
}

func driverMain() {
	port := os.Getenv("PORT")
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		logrus.Fatal("Failed to listen: ", err)
	}
	defer lis.Close()

	logrus.Infof("Listening on :%s", port)

	var server server
	var grpcServer *grpc.Server
	if tracing.IsTracingEnabled() {
		grpcServer = tracing.GetGRPCServerWithUnaryInterceptor()
	} else {
		grpcServer = grpc.NewServer()
	}
	RegisterGreeterServer(grpcServer, &server)
	reflection.Register(grpcServer)
	err = grpcServer.Serve(lis)
	if err != nil {
		logrus.Fatal("Failed to serve: ", err)
	}
}

func (s *server) SayHello(ctx context.Context, req *HelloRequest) (*HelloReply, error) {
	job := corral.NewJob(wordCount{}, wordCount{})
	options := []corral.Option{
		corral.WithSplitSize(10 * 1024),
		corral.WithMapBinSize(10 * 1024),
	}
	driver := corral.NewDriver(job, options...)
	driver.Main(ctx)
	return &HelloReply{Message: fmt.Sprintf("Hello, %s!", req.Name)}, nil
}
