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
	sdk "github.com/ease-lab/vhive-xdt/sdk/golang"
	tracing "github.com/ease-lab/vhive/utils/tracing/go"
	log "github.com/sirupsen/logrus"
	pb "tests/chained-functions-serving/proto"
)

func (s *ubenchServer) FetchByte(ctx context.Context, str *pb.ReductionRequest	) (*pb.ConsumeByteReply, error) {
	if tracing.IsTracingEnabled() {
		span1 := tracing.Span{SpanName: "custom-span-1", TracerName: "tracer"}
		span2 := tracing.Span{SpanName: "custom-span-2", TracerName: "tracer"}
		ctx = span1.StartSpan(ctx)
		ctx = span2.StartSpan(ctx)
		defer span1.EndSpan()
		defer span2.EndSpan()
	}
	if s.transferType == S3 {
		for _, capability := range str.Capability {
			log.Printf("[consumer] Consumed %d bytes\n", fetchFromS3(ctx, capability))
		}
	}else if s.transferType == XDT{
		for _, capability := range str.Capability {
			bytes, err := sdk.Get(ctx, capability, s.XDTconfig)
			if err !=nil {
				log.Printf("[consumer] Consumed %d bytes\n", len(bytes))
			}
		}
	}
	return &pb.ConsumeByteReply{Value: true}, nil
}