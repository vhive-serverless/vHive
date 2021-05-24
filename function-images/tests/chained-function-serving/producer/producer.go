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
	"math/rand"
	"net"
	"os"
	"strconv"

	"google.golang.org/grpc"

	pb "tests/chained-functions-serving/proto"
)

type producerServer struct {
	producerClient pb.ProducerConsumerClient
	pb.UnimplementedClientProducerServer
}

func (ps *producerServer) ProduceStrings(c context.Context, count *pb.ProduceStringsRequest) (*pb.Empty, error) {
	if count.Value <= 0 {
		return new(pb.Empty), nil
	} else if count.Value == 1 {
		produceSingleString(ps.producerClient, fmt.Sprint(rand.Intn(1000)))
	} else {
		wordList := make([]string, int(count.Value))
		for i := 0; i < int(count.Value); i++ {
			wordList[i] = fmt.Sprint(rand.Intn(1000))
		}
		produceStreamStrings(ps.producerClient, wordList)
	}
	return new(pb.Empty), nil
}

func produceSingleString(client pb.ProducerConsumerClient, s string) {
	ack, err := client.ConsumeString(context.Background(), &pb.ConsumeStringRequest{Value: s})
	if err != nil {
		log.Fatalf("[producer] client error in string consumption: %s", err)
	}
	log.Printf("[producer] (single) Ack: %v\n", ack.Value)
}

func produceStreamStrings(client pb.ProducerConsumerClient, strings []string) {
	//make stream
	stream, err := client.ConsumeStream(context.Background())
	if err != nil {
		log.Fatalf("[producer] %v.RecordRoute(_) = _, %v", client, err)
	}

	//stream strings
	for _, s := range strings {
		if err := stream.Send(&pb.ConsumeStringRequest{Value: s}); err != nil {
			log.Fatalf("[producer] %v.Send(%v) = %v", stream, s, err)
		}
	}

	//end transaction
	ack, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatalf("[producer] %v.CloseAndRecv() got error %v, want %v", stream, err, nil)
	}
	log.Printf("[producer] (stream) Ack: %v\n", ack.Value)
}

func main() {
	flagAddress := flag.String("addr", "localhost", "Server IP address")
	flagClientPort := flag.Int("pc", 3030, "Client Port")
	flagServerPort := flag.Int("ps", 3031, "Server Port")
	flag.Parse()

	//get address (from env or flag)
	address, ok := os.LookupEnv("ADDR")
	if !ok {
		address = *flagAddress
	}
	var clientPort int
	clientPortStr, ok := os.LookupEnv("PORT_CLIENT")
	if !ok {
		clientPort = *flagClientPort
	} else {
		var err error
		clientPort, err = strconv.Atoi(clientPortStr)
		if err != nil {
			log.Fatalf("[producer] PORT_CLIENT env variable is not int: %v", err)
		}
	}
	//get client port (from env of flag)
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

	//client setup
	log.Printf("[producer] Client using address: %v\n", address)

	conn, err := grpc.Dial(fmt.Sprintf("%v:%v", address, clientPort), grpc.WithInsecure())
	if err != nil {
		log.Fatalf("[producer] fail to dial: %s", err)
	}
	defer conn.Close()

	client := pb.NewProducerConsumerClient(conn)

	//produceSingleString(client, "hello")
	//strings := []string{"Hello", "World", "one", "two", "three"}
	//produceStreamStrings(client, strings)

	//server setup
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", serverPort))
	if err != nil {
		log.Fatalf("[producer] failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	s := producerServer{}
	s.producerClient = client
	pb.RegisterClientProducerServer(grpcServer, &s)

	log.Println("[producer] Server Started")

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("[producer] failed to serve: %s", err)
	}

}
