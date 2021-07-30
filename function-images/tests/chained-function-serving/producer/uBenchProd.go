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
	"fmt"
	"github.com/ease-lab/vhive-xdt/utils"
	log "github.com/sirupsen/logrus"
	pb_client "tests/chained-functions-serving/proto"
)

const(
	FANOUT = "FANOUT"
	FANIN = "FANIN"
	BROADCAST = "BROADCAST"
)

func (s *ubenchServer) Benchmark(ctx context.Context, benchType *pb_client.BenchType) (*pb_client.BenchResponse, error){
	addr := fmt.Sprintf("%v:%v", s.consumerAddr, s.consumerPort)
	if benchType.Name == FANOUT {
		s.fanOut(ctx, benchType.FanAmount, addr)
	}else if benchType.Name == FANIN {
		benchResponse,err := s.putData(ctx)
		return &benchResponse, err
	}
	return &pb_client.BenchResponse{Ok: false}, nil
}

func (s *ubenchServer) putData(ctx context.Context) (pb_client.BenchResponse, error) {
	if s.transferType == S3 {
		key := uploadToS3(ctx, s.payloadData)
		log.Printf("[producer] S3 push complete")
		return pb_client.BenchResponse{Capability: key, Ok: true}, nil

	} else if s.transferType == XDT {
		if capability, err := s.XDTclient.Put(s.payloadData); err != nil {
			log.Fatalf("SQP_to_dQP_data_transfer failed %v", err)
			return pb_client.BenchResponse{Ok: false}, err
		}else {
			return pb_client.BenchResponse{Capability: capability, Ok: true}, nil
		}
	}
	return pb_client.BenchResponse{Ok: false}, nil
}

func (s *ubenchServer) fanOut(ctx context.Context, fanAmount int64, addr string) pb_client.BenchResponse{
	if s.transferType == INLINE || s.transferType == S3 {
		client, conn := getGRPCclient(addr)
		defer conn.Close()
		payloadToSend := s.payloadData
		if s.transferType == S3 {
			payloadToSend = []byte(uploadToS3(ctx, s.payloadData))
		}
		for i:=int64(0); i<fanAmount; i++ {
			ack, err := client.ConsumeByte(ctx, &pb_client.ConsumeByteRequest{Value: payloadToSend})
			if err != nil {
				log.Fatalf("[producer] client error in string consumption: %s", err)
			}
			log.Printf("[producer] (single) Ack: %v\n", ack.Value)
		}
	} else if s.transferType == XDT {
		payloadToSend := utils.Payload{
			FunctionName: "HelloXDT",
			Data:         s.payloadData,
		}
		for i:=int64(0); i<fanAmount; i++ {
			if _, _, err := s.XDTclient.Invoke(addr, payloadToSend); err != nil {
				log.Fatalf("SQP_to_dQP_data_transfer failed %v", err)
			}
		}
	}
	return pb_client.BenchResponse{Ok: true}
}