// MIT License
//
// Copyright (c) 2021 Mert Bora Alper and EASE lab
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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	ctrdlog "github.com/containerd/containerd/log"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"github.com/ease-lab/vhive/utils/benchmarking/eventing/proto"
)

func usage() {
	log.Printf("Usage: %s <address> start <definition.json>", os.Args[0])
	log.Printf("Usage: %s <address> end   <results.json>", os.Args[0])
	os.Exit(1)
}

func main() {
	log.SetFlags(0)

	if len(os.Args) != 4 {
		usage()
	}

	address := os.Args[1]

	if !strings.ContainsRune(address, ':') {
		log.Fatalln("missing the port number in parameter <address>")
	}

	conn, err := grpc.Dial(address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("failed to dial: %s", err)
	}
	defer conn.Close()
	timeseriesClient := proto.NewTimeseriesClient(conn)

	switch os.Args[2] {
	case "start":
		data, err := ioutil.ReadFile(os.Args[3])
		if err != nil {
			log.Fatalf("failed to readfile: %s", err)
		}
		var experimentDefinition proto.ExperimentDefinition
		if err := json.Unmarshal(data, &experimentDefinition); err != nil {
			log.Fatalf("failed to unmarshal: %s", err)
		}
		if _, err := timeseriesClient.StartExperiment(context.Background(), &experimentDefinition); err != nil {
			log.Fatalf("failed to start experiment: %s", err)
		}
		log.Println("started experiment")

	case "end":
		res, err := timeseriesClient.EndExperiment(context.Background(), &empty.Empty{})
		if err != nil {
			log.Fatalf("failed to end experiment: %s", err)
		}

		content, err := json.MarshalIndent(res, "", "\t")
		if err != nil {
			log.Fatalf("failed to marshal: %s", err)
		}
		if err := ioutil.WriteFile(os.Args[3], content, 0666); err != nil {
			log.Fatalf("failed to writefile: %s", err)
		}

		for _, invocation := range res.Invocations {
			n, _ := fmt.Printf("INVOCATION {%s}\n", invocation.Id)
			fmt.Println(strings.Repeat("=", n - 1))
			fmt.Printf("Id       : %s\n", invocation.Id)
			fmt.Printf("InvokedOn: %s\n", invocation.InvokedOn.AsTime().Format(ctrdlog.RFC3339NanoFixed))
			fmt.Printf("Duration : %dms\n", invocation.Duration.AsDuration().Milliseconds())
			fmt.Printf("Status   : %s\n", invocation.Status.String())
			fmt.Printf("\n\n")
		}
		if len(res.Invocations) == 0 {
			fmt.Println("no invocations has been found")
		}

	default:
		usage()
	}
}

