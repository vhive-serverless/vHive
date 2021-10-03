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

// Package tracing provides a simple utility for including opentelemetry and zipkin tracing
// instrumentation in vHive and Knative workloads

package storage

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	m.Run()
}

func TestS3(t *testing.T) {
	InitStorage("S3", "go-storage-test")
	msg := []byte("test message")
	Put("testkey", msg)
	ret := Get("testkey")
	if string(ret) != "test message" {
		t.Errorf("Get() recieved wrong string: \"%s\"", string(ret))
	}
}

func TestS3File(t *testing.T) {
	InitStorage("S3", "go-storage-test")
	file, err := os.Open("testFile.txt")
	if err != nil {
		t.Errorf("Test file could not be read")
	}
	fileContent, _ := os.ReadFile("testFile.txt")
	PutFile("testkey", file)
	ret := Get("testkey")
	if string(ret) != string(fileContent) {
		t.Errorf("Get() recieved wrong string: \"%s\"", string(ret))
	}
}

func TestElasticache(t *testing.T) {
	InitStorage("ELASTICACHE", "test4.0vgvbw.ng.0001.usw1.cache.amazonaws.com:6379")
	msg := []byte("test message")
	Put("testkey", msg)
	ret := Get("testkey")
	if string(ret) != "test message" {
		t.Errorf("Get() recieved wrong string: \"%s\"", string(ret))
	}
}
