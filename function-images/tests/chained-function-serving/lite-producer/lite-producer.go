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
	"log"
	"net"
	"os"
	"strconv"

	"google.golang.org/grpc"

	pb "tests/chained-functions-serving/proto"
)

type producerServer struct {
	pb.UnimplementedClientProducerServer
}

func (ps *producerServer) ProduceStrings(c context.Context, count *pb.ProduceStringsRequest) (*pb.Empty, error) {
	log.Println("client successful call")
	return new(pb.Empty), nil
}

func main() {
	flagServerPort := flag.Int("ps", 3031, "Server Port")
	flag.Parse()

	var serverPort int
	serverPortStr, ok := os.LookupEnv("PORT")
	if !ok {
		serverPort = *flagServerPort
	} else {
		var err error
		serverPort, err = strconv.Atoi(serverPortStr)
		if err != nil {
			log.Fatalf("[producer] PORT env variable is not int: %v", err)
		}
	}

	//set up log file
	file, err := os.OpenFile("log/logs.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	log.SetOutput(file)

	//server setup
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", serverPort))
	if err != nil {
		log.Fatalf("[producer] failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	s := producerServer{}
	pb.RegisterClientProducerServer(grpcServer, &s)

	log.Println("[producer] Server Started")

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("[producer] failed to serve: %s", err)
	}

}
