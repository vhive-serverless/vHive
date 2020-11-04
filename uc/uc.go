package main

import (
	"context"
	"log"
	"net"
	"os"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"

	"github.com/pkg/errors"
	hpb "github.com/plamenmpetrov/helloworld-proto"
)

const (
	guestIPEnv   = "GUESTIP"
	guestPortEnv = "GUESTPORT"
	defaultPort  = "50051"
)

var (
	guestIP, guestPort string
	guestClient        hpb.GreeterClient
)

type server struct {
	hpb.UnimplementedGreeterServer
}

func main() {
	guestIP = os.Getenv(guestIPEnv)
	guestPort = os.Getenv(guestPortEnv)

	if guestIP == "" {
		log.Fatal("Must provide guest IP")
	}

	if guestPort == "" {
		guestPort = defaultPort
	}

	// Get Guest client
	funcClient, err := getFuncClient(guestIP, guestPort)
	if err != nil {
		log.Panic("failed to get guest func client")
	}
	guestClient = funcClient

	// Create forwading server
	lis, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	hpb.RegisterGreeterServer(s, &server{})

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func (s *server) SayHello(ctx context.Context, req *hpb.HelloRequest) (*hpb.HelloReply, error) {
	return guestClient.SayHello(ctx, req)
}

func getFuncClient(guestIP, guestPort string) (hpb.GreeterClient, error) {
	backoffConfig := backoff.DefaultConfig
	backoffConfig.MaxDelay = 5 * time.Second
	connParams := grpc.ConnectParams{
		Backoff: backoffConfig,
	}

	gopts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithInsecure(),
		grpc.FailOnNonTempDialError(true),
		grpc.WithConnectParams(connParams),
		grpc.WithContextDialer(contextDialer),
	}

	//  This timeout must be large enough for all functions to start up (e.g., ML training takes few seconds)
	ctxx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctxx, guestIP+":"+guestPort, gopts...)
	if err != nil {
		return nil, err
	}
	return hpb.NewGreeterClient(conn), nil
}

func contextDialer(ctx context.Context, address string) (net.Conn, error) {
	if deadline, ok := ctx.Deadline(); ok {
		return timeoutDialer(address, time.Until(deadline))
	}
	return timeoutDialer(address, 0)
}

func isConnRefused(err error) bool {
	if err != nil {
		if nerr, ok := err.(*net.OpError); ok {
			if serr, ok := nerr.Err.(*os.SyscallError); ok {
				if serr.Err == syscall.ECONNREFUSED {
					return true
				}
			}
		}
	}
	return false
}

type dialResult struct {
	c   net.Conn
	err error
}

func timeoutDialer(address string, timeout time.Duration) (net.Conn, error) {
	var (
		stopC = make(chan struct{})
		synC  = make(chan *dialResult)
	)
	go func() {
		defer close(synC)
		for {
			select {
			case <-stopC:
				return
			default:
				c, err := net.DialTimeout("tcp", address, timeout)
				if isConnRefused(err) {
					<-time.After(1 * time.Millisecond)
					continue
				}
				if err != nil {
					<-time.After(1 * time.Millisecond)
					continue
				}

				synC <- &dialResult{c, err}
				return
			}
		}
	}()
	select {
	case dr := <-synC:
		return dr.c, dr.err
	case <-time.After(timeout):
		close(stopC)
		go func() {
			dr := <-synC
			if dr != nil && dr.c != nil {
				dr.c.Close()
			}
		}()
		return nil, errors.Errorf("dial %s: timeout", address)
	}
}
