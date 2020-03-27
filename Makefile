fccd: proto
	go install github.com/ustiugov/fccd-orchestrator

protobuf:
	protoc -I proto/ proto/orchestrator.proto --go_out=plugins=grpc:proto
	protoc -I ../workloads/protos ../workloads/protos/helloworld.proto --go_out=plugins=grpc:helloworld

clean:
	rm proto/orchestrator.pb.go
