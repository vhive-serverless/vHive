package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	pb "github.com/ease-lab/vhive/examples/protobuf/helloworld"
	tracing "github.com/ease-lab/vhive/utils/tracing/go"
)

var (
	completed   int64
	latSlice    LatencySlice
	portFlag    *int
	grpcTimeout time.Duration
	withTracing *bool
)

func main() {
	endpoint := flag.String("endpoint", "", "Endpoint to ping")
	sampleSize := flag.Int64("sampleSize", 10, "Number of samples")
	latencyOutputFile := flag.String("latf", "lat.csv", "CSV file for the latency measurements in microseconds")
	portFlag = flag.Int("port", 80, "The port that functions listen to")
	withTracing = flag.Bool("trace", false, "Enable tracing in the client")
	zipkin := flag.String("zipkin", "http://localhost:9411/api/v2/spans", "zipkin url")
	debug := flag.Bool("dbg", false, "Enable debug logging")
	grpcTimeout = time.Duration(*flag.Int("grpcTimeout", 30, "Timeout in seconds for gRPC requests")) * time.Second

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

	if *withTracing {
		shutdown, err := tracing.InitBasicTracer(*zipkin, "invoker")
		if err != nil {
			log.Print(err)
		}
		defer shutdown()
	}

	runExperiment(*endpoint, *sampleSize)

	writeLatencies(*sampleSize, *latencyOutputFile)
}

func runExperiment(endpoint string, sampleSize int64) {
	var i int64
	for i = 0; i < sampleSize; i++ {
		invokeServingFunction(endpoint)
	}
	log.Println("Experiment finished!")

}

func SayHello(address string) {
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

	ctx, cancel := context.WithTimeout(context.Background(), grpcTimeout)
	defer cancel()

	_, err = c.SayHello(ctx, &pb.HelloRequest{Name: "faas"})
	if err != nil {
		log.Warnf("Failed to invoke %v, err=%v", address, err)
	}
}

func invokeServingFunction(endpoint string) {
	defer getDuration(startMeasurement(endpoint)) // measure entire invocation time

	address := fmt.Sprintf("%s:%d", endpoint, *portFlag)
	log.Debug("Invoking by the address: %v", address)

	SayHello(address)

	atomic.AddInt64(&completed, 1)

	return
}

// LatencySlice is a thread-safe slice to hold a slice of latency measurements.
type LatencySlice struct {
	sync.Mutex
	slice []int64
}

func startMeasurement(msg string) (string, time.Time) {
	return msg, time.Now()
}

func getDuration(msg string, start time.Time) {
	latency := time.Since(start)
	log.Debugf("Invoked %v in %v usec\n", msg, latency.Microseconds())
	addDurations([]time.Duration{latency})
}

func addDurations(ds []time.Duration) {
	latSlice.Lock()
	for _, d := range ds {
		latSlice.slice = append(latSlice.slice, d.Microseconds())
	}
	latSlice.Unlock()
}

func writeLatencies(sampleSize int64, latencyOutputFile string) {
	latSlice.Lock()
	defer latSlice.Unlock()

	fileName := fmt.Sprintf("%d_%s", sampleSize, latencyOutputFile)
	log.Info("The measured latencies are saved in ", fileName)

	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)

	if err != nil {
		log.Fatal("Failed creating file: ", err)
	}

	datawriter := bufio.NewWriter(file)

	for _, lat := range latSlice.slice {
		_, err := datawriter.WriteString(strconv.FormatInt(lat, 10) + "\n")
		if err != nil {
			log.Fatal("Failed to write the latencies to a file ", err)
		}
	}

	datawriter.Flush()
	file.Close()
	return
}
