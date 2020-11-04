package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	hpb "github.com/ustiugov/fccd-orchestrator/helloworld"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var (
	serverAddr = flag.String("server_addr", "f1.default.192.168.1.240.xip.io:80", "The server address in the format of host:port")
)

func main() {
	flag.Parse()

	var opts []grpc.DialOption

	opts = append(opts, grpc.WithInsecure())

	conn, err := grpc.Dial(*serverAddr, opts...)
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()

	client := hpb.NewGreeterClient(conn)

	ctxFwd, cancel := context.WithDeadline(context.Background(), time.Now().Add(20*time.Second))
	defer cancel()

	resp, err := client.SayHello(ctxFwd, &hpb.HelloRequest{Name: "record"})
	if err != nil {
		fmt.Printf("error %v\n", err)
		return
	}

	fmt.Printf("Response is %s\n", resp.Message)
}
