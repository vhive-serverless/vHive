invoker: client.go measure.go helloworld.pb.go helloworld_grpc.pb.go
	go build github.com/ease-lab/vhive/examples/invoker

helloworld.pb.go: helloworld.proto
	protoc \
		--go_out=. \
		--go_opt="paths=source_relative" \
		helloworld.proto

helloworld_grpc.pb.go: helloworld.proto
	protoc \
		--go-grpc_out=. \
		--go-grpc_opt="paths=source_relative" \
		helloworld.proto