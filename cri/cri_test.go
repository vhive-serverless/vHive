// MIT License
//
// Copyright (c) 2020 Plamen Petrov
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

package cri

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"testing"

	"sync"
	"time"

	"github.com/stretchr/testify/require"

	hpb "github.com/ustiugov/fccd-orchestrator/helloworld"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var functionURL string = "f1.default.192.168.1.240.xip.io:80"

func TestSingleCall(t *testing.T) {
	invoke(t, functionURL)
}

func TestParallelCall(t *testing.T) {
	n := 50
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			invoke(t, functionURL)
		}()
	}
	wg.Wait()
}

func TestBench(t *testing.T) {
	dropPageCache()
	start := time.Now()
	invoke(t, functionURL)
	end := time.Since(start)
	fmt.Printf("First invocation took %d ms\n", end.Milliseconds())

	dropPageCache()
	start = time.Now()
	invoke(t, functionURL)
	end = time.Since(start)
	fmt.Printf("Second invocation took %d ms\n", end.Milliseconds())
}

func invoke(t *testing.T, functionURL string) {
	client, conn, err := getClient(functionURL)
	require.NoError(t, err, "Failed to dial function URL")
	defer conn.Close()
	ctxFwd, cancel := context.WithDeadline(context.Background(), time.Now().Add(20*time.Second))
	defer cancel()

	_, err = client.SayHello(ctxFwd, &hpb.HelloRequest{Name: "record"})
	require.NoError(t, err, "Failed to get response from function")
}

func getClient(functionURL string) (hpb.GreeterClient, *grpc.ClientConn, error) {
	conn, err := grpc.Dial(functionURL, grpc.WithInsecure())
	if err != nil {
		return nil, nil, err
	}
	return hpb.NewGreeterClient(conn), conn, nil
}

func dropPageCache() {
	cmd := exec.Command("sudo", "/bin/sh", "-c", "sync; echo 1 > /proc/sys/vm/drop_caches")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to drop caches: %v", err)
	}
}
